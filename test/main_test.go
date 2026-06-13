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
	"testing"

	"github.com/alpody/fiber-realworld/db"
	"github.com/alpody/fiber-realworld/handler"
	"github.com/alpody/fiber-realworld/router"
	"github.com/alpody/fiber-realworld/store"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
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

// testApp is one fully-wired, isolated instance of the application.
type testApp struct {
	app *fiber.App
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
	return &testApp{app: app}
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
	resp := ta.doReq(t, http.MethodPost, "/api/users", payload, "")
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
