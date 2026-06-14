package test

import (
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/stretchr/testify/require"
)

// ---- Add comment ---------------------------------------------------------

func TestComments_Add_ByAuthenticatedUser(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var token string
		given("an authenticated user and an article", func() {
			token = ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Commentable")
		})

		var out commentResp
		var resp apiResp
		when("she POSTs a comment on the article", func() {
			resp = ta.doReq(t, http.MethodPost, pathComments(created.Article.Slug),
				map[string]any{"comment": map[string]string{"body": "great read"}}, token)
			attach("response", resp)
		})
		then("the response is 201 Created", func() {
			require.Equal(t, http.StatusCreated, resp.status)
			decode(t, resp, &out)
		})
		and("the comment body, author and a generated id are returned", func() {
			require.Equal(t, "great read", out.Comment.Body)
			require.Equal(t, "alice", out.Comment.Author.Username)
			require.NotZero(t, out.Comment.ID)
		})
	}, allure.Feature(featComments), allure.Story(storyCreate),
		allure.Description("An authenticated user can comment on an article (201)"),
		allure.Severity(sevCritical))
}

func TestComments_Add_Unauthenticated(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		given("an existing article", func() {
			token := ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Commentable")
		})

		var resp apiResp
		when("a comment is POSTed with no token", func() {
			resp = ta.doReq(t, http.MethodPost, pathComments(created.Article.Slug),
				map[string]any{"comment": map[string]string{"body": "no auth"}}, "")
			attach("response", resp)
		})
		then("the JWT middleware rejects it with 400", func() {
			require.Equal(t, http.StatusBadRequest, resp.status)
		})
	}, allure.Feature(featComments), allure.Story(storyCreate),
		allure.Description("Commenting without a token is rejected by the JWT middleware (400)"),
		allure.Severity(sevNormal))
}

func TestComments_Add_ToNonExistentArticle(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var token string
		given("an authenticated user", func() {
			token = ta.registerAndLogin(t, "alice")
		})

		var resp apiResp
		when("she comments on an article slug that does not exist", func() {
			resp = ta.doReq(t, http.MethodPost, pathComments("no-such-article"),
				map[string]any{"comment": map[string]string{"body": "hi"}}, token)
			attach("response", resp)
		})
		then("the response is 404 Not Found", func() {
			require.Equal(t, http.StatusNotFound, resp.status)
		})
	}, allure.Feature(featComments), allure.Story(storyCreate),
		allure.Description("Commenting on a non-existent article returns 404"),
		allure.Severity(sevNormal))
}

// ---- Read comments -------------------------------------------------------

func TestComments_Get_Public(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		given("an article with two comments", func() {
			token := ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Readable")
			ta.addComment(t, token, created.Article.Slug, "first")
			ta.addComment(t, token, created.Article.Slug, "second")
		})

		var out struct {
			Comments []struct {
				Body string `json:"body"`
			} `json:"comments"`
		}
		var resp apiResp
		when("the comment list is GETed with no auth", func() {
			resp = ta.doReq(t, http.MethodGet, pathComments(created.Article.Slug), nil, "")
			attach("response", resp)
		})
		then("the response is 200 OK", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("both comments are returned", func() {
			require.Len(t, out.Comments, 2)
		})
	}, allure.Feature(featComments), allure.Story(storyRead),
		allure.Description("Reading an article's comments is public and returns the full list"),
		allure.Severity(sevNormal))
}

// ---- Delete comment ------------------------------------------------------

func TestComments_Delete_ByAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var cm commentResp
		var token string
		given("an article with a comment by alice", func() {
			token = ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Deletable Comments")
			cm = ta.addComment(t, token, created.Article.Slug, "delete me")
		})

		when("the comment's author DELETEs it", func() {
			resp := ta.doReq(t, http.MethodDelete,
				pathComment(created.Article.Slug, itoa(cm.Comment.ID)), nil, token)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
		})
		then("the comment list is now empty", func() {
			resp := ta.doReq(t, http.MethodGet, pathComments(created.Article.Slug), nil, "")
			var out struct {
				Comments []struct {
					ID uint `json:"id"`
				} `json:"comments"`
			}
			decode(t, resp, &out)
			require.Empty(t, out.Comments)
		})
	}, allure.Feature(featComments), allure.Story(storyDelete),
		allure.Description("The comment's author can delete it; it then disappears from the list"),
		allure.Severity(sevCritical))
}

func TestComments_Delete_ByNonAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var cm commentResp
		var bob string
		given("a comment by alice and a second user bob", func() {
			alice := ta.registerAndLogin(t, "alice")
			bob = ta.registerAndLogin(t, "bob")
			created = ta.createArticle(t, alice, "Alice's Article")
			cm = ta.addComment(t, alice, created.Article.Slug, "alice's comment")
		})

		var resp apiResp
		when("bob tries to DELETE alice's comment", func() {
			resp = ta.doReq(t, http.MethodDelete,
				pathComment(created.Article.Slug, itoa(cm.Comment.ID)), nil, bob)
			attach("response", resp)
		})
		then("it is rejected with 401 (the handler checks comment ownership)", func() {
			require.Equal(t, http.StatusUnauthorized, resp.status)
		})
	}, allure.Feature(featComments), allure.Story(storyDelete),
		allure.Description("A non-author cannot delete someone else's comment (401)"),
		allure.Severity(sevCritical))
}

func TestComments_Delete_NonNumericID(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var token string
		given("an authenticated user and an article", func() {
			token = ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Bad ID")
		})

		var resp apiResp
		when("a DELETE uses a non-numeric comment id", func() {
			resp = ta.doReq(t, http.MethodDelete,
				pathComment(created.Article.Slug, "not-a-number"), nil, token)
			attach("response", resp)
		})
		then("it is rejected with 400 before any store lookup (strconv.ParseUint fails)", func() {
			require.Equal(t, http.StatusBadRequest, resp.status)
		})
	}, allure.Feature(featComments), allure.Story(storyDelete),
		allure.Description("A non-numeric comment id is rejected with 400"),
		allure.Severity(sevNormal))
}

func TestComments_Delete_Missing(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var token string
		given("an authenticated user and an article with no comments", func() {
			token = ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Missing Comment")
		})

		var resp apiResp
		when("she DELETEs a comment id that does not exist", func() {
			resp = ta.doReq(t, http.MethodDelete,
				pathComment(created.Article.Slug, "99999"), nil, token)
			attach("response", resp)
		})
		then("the response is 404 Not Found", func() {
			require.Equal(t, http.StatusNotFound, resp.status)
		})
	}, allure.Feature(featComments), allure.Story(storyDelete),
		allure.Description("Deleting a non-existent comment id returns 404"),
		allure.Severity(sevNormal))
}

// TestComments_Delete_CrashesOnDBError_SPEC documents bug #2: DeleteComment
// handles a datastore error from GetCommentByID with `log.Fatal(err)` (see
// handler/article.go) instead of returning HTTP 500. log.Fatal calls os.Exit,
// so a single failing DB read in this handler kills the entire server process,
// tearing down every other in-flight request — a denial-of-service waiting to
// happen.
//
// The error path is unreachable through ordinary input (a missing comment is
// nil,nil → 404), so we provoke a real datastore error by closing the DB, then
// observe that the process dies. The whole thing runs in a forked child so the
// crash doesn't take the test runner down with it; the parent asserts the child
// survived.
//
// FAILS until bug #2 is fixed (handler should return 500, not call log.Fatal).
func TestComments_Delete_CrashesOnDBError_SPEC(t *testing.T) {
	if os.Getenv("BUG2_CRASH_CHILD") == "1" {
		// Child: drive DeleteComment into its error branch. If the handler is
		// fixed it returns 500 and we exit 0; today it log.Fatals and exits 1.
		ta := newApp(t)
		token := ta.registerAndLogin(t, "victim")
		art := ta.createArticle(t, token, "Boom")
		ta.addComment(t, token, art.Article.Slug, "doomed")
		ta.closeDB(t)
		ta.doReq(t, http.MethodDelete, pathComment(art.Article.Slug, "1"), nil, token)
		return
	}

	t.Parallel()
	runTest(t, func() {
		var out []byte
		var err error
		when("a datastore error occurs during DeleteComment (forked child: DB closed mid-request)", func() {
			cmd := exec.Command(os.Args[0], "-test.run=^TestComments_Delete_CrashesOnDBError_SPEC$")
			cmd.Env = append(os.Environ(), "BUG2_CRASH_CHILD=1")
			out, err = cmd.CombinedOutput()
			_ = allure.AddAttachment("child output", allure.TextPlain, out)
		})
		then("the server must survive and return 500 — but BUG #2: log.Fatal kills the process (child exits non-zero)", func() {
			require.NoError(t, err,
				"BUG #2: a datastore error in DeleteComment called log.Fatal and killed the process "+
					"(child exit: %v); the handler must return HTTP 500 instead of crashing the server", err)
		})
	}, allure.Feature(featComments), allure.Story(storyDelete),
		allure.Description("SPEC: a datastore error must yield 500, not crash the process. Documents bug #2 — runs RED."),
		allure.Tag(tagBug2), allure.Tag(tagSpec), allure.Severity(sevBlocker))
}
