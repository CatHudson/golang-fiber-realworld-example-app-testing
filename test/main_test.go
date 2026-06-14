// Package test holds the service-level / integration suite. It drives the full
// app in-process (real Fiber router + middleware + JWT, real GORM, real in-memory
// SQLite) over HTTP via fiber's app.Test(...). Nothing is mocked — this is where
// the app's real behaviour, and its bugs, surface.
//
// The suite is plain `go test` (no build tag): it's backed by in-memory SQLite
// and runs in milliseconds per test, so it always runs in CI alongside the unit
// layer. There's no value in making a fast, always-relevant suite opt-in.
package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/alpody/fiber-realworld/db"
	"github.com/alpody/fiber-realworld/handler"
	"github.com/alpody/fiber-realworld/router"
	"github.com/alpody/fiber-realworld/store"
	"github.com/dailymotion/allure-go"
	"github.com/dailymotion/allure-go/severity"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// API route paths. Static paths are constants; resource paths that take a
// parameter are built by the helpers below so the "/api/..." prefix lives in
// exactly one place.
const (
	pathUsers    = "/api/users"
	pathLogin    = "/api/users/login"
	pathUser     = "/api/user"
	pathArticles = "/api/articles"
)

func pathArticle(slug string) string     { return pathArticles + "/" + slug }
func pathComments(slug string) string    { return pathArticle(slug) + "/comments" }
func pathComment(slug, id string) string { return pathComments(slug) + "/" + id }
func pathFavorite(slug string) string    { return pathArticle(slug) + "/favorite" }

// Allure organisational labels — Epic / Feature / Story / Tag. Centralised so
// the report taxonomy stays consistent and renames happen in one place.
const (
	epicAPI = "Conduit API"

	featAuth      = "Auth"
	featArticles  = "Articles"
	featComments  = "Comments"
	featFavorites = "Favorites"

	storyRegistration = "Registration"
	storyLogin        = "Login"
	storyValidation   = "Validation"
	storyCurrentUser  = "Current user"
	storyCreate       = "Create"
	storyRead         = "Read"
	storyUpdate       = "Update"
	storyDelete       = "Delete"
	storyFilter       = "Filtering & pagination"
	storyFavorite     = "Favorite"
	storyUnfavorite   = "Unfavorite"

	tagSpec = "spec"
	tagBug1 = "bug-1"
	tagBug2 = "bug-2"
	tagBug3 = "bug-3"
	tagBug5 = "bug-5"
)

// runTest wraps a test body in an Allure test node with the common Epic and any
// extra labels (Feature/Story/Description/Severity/Tag). Keeps each test file
// focused on behaviour rather than Allure boilerplate.
func runTest(t *testing.T, body func(), opts ...allure.Option) {
	t.Helper()
	all := append([]allure.Option{allure.Epic(epicAPI), allure.Action(body)}, opts...)
	allure.Test(t, all...)
}

// itoa formats an unsigned id (e.g. a comment ID) for use in a URL path.
func itoa(n uint) string { return strconv.FormatUint(uint64(n), 10) }

// step is a thin wrapper around allure.Step for readability.
func step(desc string, fn func()) {
	allure.Step(allure.Description(desc), allure.Action(fn))
}

// attach records a response body as a JSON attachment on the current step/test —
// used on bug-probing assertions so the failing payload is visible in the report.
func attach(name string, r apiResp) {
	_ = allure.AddAttachment(name, allure.ApplicationJson, r.body)
}

// sev re-exports the severity levels actually used, to keep imports tidy in tests.
var (
	sevBlocker  = severity.Blocker
	sevCritical = severity.Critical
	sevNormal   = severity.Normal
)

// TestMain gives allure-go a writable results directory by default.
//
// allure-go writes to "$ALLURE_RESULTS_PATH/allure-results". When the variable
// is unset it falls back to the absolute "/allure-results", which fails to
// create on a read-only root and floods the run with errors. Defaulting it to
// the current working directory means a plain local `go test` produces
// ./allure-results with no setup. CI sets the variable explicitly (to the repo
// root) and that value takes precedence.
func TestMain(m *testing.M) {
	if os.Getenv("ALLURE_RESULTS_PATH") == "" {
		if wd, err := os.Getwd(); err == nil {
			_ = os.Setenv("ALLURE_RESULTS_PATH", wd)
		}
	}
	os.Exit(m.Run())
}

// testApp is one fully-wired, isolated instance of the application. It keeps a
// handle on the underlying *gorm.DB so a test can deliberately break the
// datastore (e.g. closeDB) to exercise error paths.
type testApp struct {
	app *fiber.App
	db  *gorm.DB
}

// newApp builds a fresh app backed by its own in-memory database. Each call is
// fully isolated from every other — no shared files, no cross-test state.
func newApp(t *testing.T) *testApp {
	t.Helper()
	d := db.TestDB()
	db.AutoMigrate(d)
	us := store.NewUserStore(d)
	as := store.NewArticleStore(d)
	h := handler.NewHandler(us, as)
	app := router.New()
	h.Register(app)
	return &testApp{app: app, db: d}
}

// closeDB closes the underlying sql.DB so subsequent store calls fail with
// "sql: database is closed" — used to drive handler error branches.
func (ta *testApp) closeDB(t *testing.T) {
	t.Helper()
	sqlDB, err := ta.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// apiResp is a decoded HTTP response: status + raw body bytes.
type apiResp struct {
	status int
	body   []byte
}

// doReq sends a request through the real router and returns status + body.
// body may be nil (no payload) or any JSON-marshalable value. token, if
// non-empty, is sent as the "Token <token>" Authorization header.
func (ta *testApp) doReq(t *testing.T, method, target string, body any, token string) apiResp {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Token "+token)
	}
	// -1 disables the request timeout so a crash/hang surfaces as a test
	// failure rather than a flaky timeout.
	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return apiResp{status: resp.StatusCode, body: data}
}

// decode unmarshals a response body into v, failing the test on malformed JSON.
func decode(t *testing.T, r apiResp, v any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(r.body, v), "decoding response body: %s", string(r.body))
}

// userResp is the shape of the {"user": {...}} envelope returned by auth endpoints.
type userResp struct {
	User struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Token    string `json:"token"`
	} `json:"user"`
}

// register creates a new user via the public API and returns the full response.
func (ta *testApp) register(t *testing.T, username string) (userResp, apiResp) {
	t.Helper()
	payload := map[string]any{"user": map[string]string{
		"username": username,
		"email":    username + "@realworld.io",
		"password": "secret123",
	}}
	resp := ta.doReq(t, http.MethodPost, pathUsers, payload, "")
	var out userResp
	if resp.status == http.StatusCreated {
		decode(t, resp, &out)
	}
	return out, resp
}

// registerAndLogin registers a user and returns a usable JWT token. Registration
// already returns a token, so a separate login round-trip isn't required.
func (ta *testApp) registerAndLogin(t *testing.T, username string) string {
	t.Helper()
	out, resp := ta.register(t, username)
	require.Equal(t, http.StatusCreated, resp.status, "register %q failed: %s", username, string(resp.body))
	require.NotEmpty(t, out.User.Token)
	return out.User.Token
}

// login posts credentials to /api/users/login and returns the raw response.
func (ta *testApp) login(t *testing.T, email, password string) apiResp {
	t.Helper()
	return ta.doReq(t, http.MethodPost, pathLogin,
		map[string]any{"user": map[string]string{"email": email, "password": password}}, "")
}

// articleResp is the {"article": {...}} envelope returned by article endpoints.
type articleResp struct {
	Article struct {
		Slug           string   `json:"slug"`
		Title          string   `json:"title"`
		Description    string   `json:"description"`
		Body           string   `json:"body"`
		TagList        []string `json:"tagList"`
		Favorited      bool     `json:"favorited"`
		FavoritesCount int      `json:"favoritesCount"`
		Author         struct {
			Username  string `json:"username"`
			Following bool   `json:"following"`
		} `json:"author"`
	} `json:"article"`
}

// createArticle publishes an article as the holder of token and returns the
// decoded response. Fails the test if creation doesn't return 201.
func (ta *testApp) createArticle(t *testing.T, token, title string, tags ...string) articleResp {
	t.Helper()
	art := map[string]any{
		"title":       title,
		"description": "description of " + title,
		"body":        "body of " + title,
	}
	if len(tags) > 0 {
		art["tagList"] = tags
	}
	resp := ta.doReq(t, http.MethodPost, pathArticles, map[string]any{"article": art}, token)
	require.Equal(t, http.StatusCreated, resp.status, "createArticle %q failed: %s", title, string(resp.body))
	var out articleResp
	decode(t, resp, &out)
	return out
}

// commentResp is the {"comment": {...}} envelope returned by comment endpoints.
type commentResp struct {
	Comment struct {
		ID     uint   `json:"id"`
		Body   string `json:"body"`
		Author struct {
			Username string `json:"username"`
		} `json:"author"`
	} `json:"comment"`
}

// addComment posts a comment on slug as the holder of token and returns the
// decoded response. Fails the test if creation doesn't return 201.
func (ta *testApp) addComment(t *testing.T, token, slug, body string) commentResp {
	t.Helper()
	resp := ta.doReq(t, http.MethodPost, pathComments(slug),
		map[string]any{"comment": map[string]string{"body": body}}, token)
	require.Equal(t, http.StatusCreated, resp.status, "addComment failed: %s", string(resp.body))
	var out commentResp
	decode(t, resp, &out)
	return out
}

// articleListResp is the {"articles": [...], "articlesCount": N} envelope
// returned by the article list endpoint.
type articleListResp struct {
	Articles []struct {
		Slug    string   `json:"slug"`
		Title   string   `json:"title"`
		TagList []string `json:"tagList"`
		Author  struct {
			Username string `json:"username"`
		} `json:"author"`
	} `json:"articles"`
	ArticlesCount int64 `json:"articlesCount"`
}

// listArticles GETs /api/articles with the given query string (e.g. "?tag=go")
// and returns the decoded list plus the raw response. The endpoint is public.
func (ta *testApp) listArticles(t *testing.T, query string) (articleListResp, apiResp) {
	t.Helper()
	resp := ta.doReq(t, http.MethodGet, pathArticles+query, nil, "")
	require.Equal(t, http.StatusOK, resp.status, "list failed: %s", string(resp.body))
	var out articleListResp
	decode(t, resp, &out)
	return out, resp
}

// slugs extracts the article slugs from a list response, in order.
func (r articleListResp) slugs() []string {
	out := make([]string, len(r.Articles))
	for i, a := range r.Articles {
		out[i] = a.Slug
	}
	return out
}
