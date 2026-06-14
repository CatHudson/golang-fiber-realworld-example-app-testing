package test

import (
	"net/http"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/stretchr/testify/require"
)

func TestArticles_Create_Success(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")

		var out articleResp
		step("POST /api/articles as an authenticated author", func() {
			resp := ta.doReq(t, http.MethodPost, pathArticles, map[string]any{"article": map[string]any{
				"title":       "How to Train Your Dragon",
				"description": "ever wonder how?",
				"body":        "very carefully.",
				"tagList":     []string{"dragons", "training"},
			}}, token)
			attach("response", resp)
			require.Equal(t, http.StatusCreated, resp.status)
			decode(t, resp, &out)
		})
		require.Equal(t, "how-to-train-your-dragon", out.Article.Slug, "slug should be derived from the title")
		require.Equal(t, "alice", out.Article.Author.Username)
		require.ElementsMatch(t, []string{"dragons", "training"}, out.Article.TagList)
	}, allure.Feature(featArticles), allure.Story(storyCreate),
		allure.Description("An authenticated user can create an article; the slug is derived from the title"),
		allure.Severity(sevCritical))
}

func TestArticles_Create_Unauthenticated(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		resp := ta.doReq(t, http.MethodPost, pathArticles, map[string]any{"article": map[string]any{
			"title": "No Auth", "description": "d", "body": "b",
		}}, "")
		attach("response", resp)
		// Non-GET article routes require JWT; missing token → 400 from middleware.
		require.Equal(t, http.StatusBadRequest, resp.status)
	}, allure.Feature(featArticles), allure.Story(storyCreate),
		allure.Description("Creating an article without a token is rejected by the JWT middleware"),
		allure.Severity(sevNormal))
}

func TestArticles_GetBySlug_Public(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")
		created := ta.createArticle(t, token, "Public Read")

		var out articleResp
		step("GET the article by slug with no auth (public read)", func() {
			resp := ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, "")
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		require.Equal(t, created.Article.Slug, out.Article.Slug)
		require.Equal(t, "alice", out.Article.Author.Username)
	}, allure.Feature(featArticles), allure.Story(storyRead),
		allure.Description("Reading an article by slug is public and returns 200"),
		allure.Severity(sevNormal))
}

func TestArticles_GetBySlug_NotFound(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		resp := ta.doReq(t, http.MethodGet, pathArticle("no-such-article"), nil, "")
		attach("response", resp)
		require.Equal(t, http.StatusNotFound, resp.status)
	}, allure.Feature(featArticles), allure.Story(storyRead),
		allure.Description("Reading a non-existent slug returns 404"),
		allure.Severity(sevNormal))
}

func TestArticles_Update_ByAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")
		created := ta.createArticle(t, token, "Original Title")

		var out articleResp
		step("PUT the article as its author", func() {
			resp := ta.doReq(t, http.MethodPut, pathArticle(created.Article.Slug), map[string]any{"article": map[string]any{
				"title": "Updated Title",
				"body":  "updated body",
			}}, token)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		require.Equal(t, "Updated Title", out.Article.Title)
		require.Equal(t, "updated-title", out.Article.Slug, "slug should be regenerated from the new title")
	}, allure.Feature(featArticles), allure.Story(storyUpdate),
		allure.Description("The author can update their article; the slug is regenerated"),
		allure.Severity(sevCritical))
}

func TestArticles_Update_ByNonAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		alice := ta.registerAndLogin(t, "alice")
		bob := ta.registerAndLogin(t, "bob")
		created := ta.createArticle(t, alice, "Alice's Article")

		var resp apiResp
		step("PUT Alice's article while authenticated as Bob", func() {
			resp = ta.doReq(t, http.MethodPut, pathArticle(created.Article.Slug), map[string]any{"article": map[string]any{
				"title": "Hijacked",
			}}, bob)
			attach("response", resp)
		})
		// GetUserArticleBySlug filters by author id, so a non-author sees the
		// article as non-existent → 404 (the write is correctly prevented).
		require.Equal(t, http.StatusNotFound, resp.status)

		step("Confirm the article was not modified", func() {
			check := ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, "")
			var out articleResp
			decode(t, check, &out)
			require.Equal(t, "Alice's Article", out.Article.Title)
		})
	}, allure.Feature(featArticles), allure.Story(storyUpdate),
		allure.Description("A non-author cannot update someone else's article (404) and the article is unchanged"),
		allure.Severity(sevCritical))
}

func TestArticles_Delete_ByAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")
		created := ta.createArticle(t, token, "Delete Me")

		step("DELETE the article as its author", func() {
			resp := ta.doReq(t, http.MethodDelete, pathArticle(created.Article.Slug), nil, token)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
		})
		step("Confirm it is gone (404 on read)", func() {
			resp := ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, "")
			require.Equal(t, http.StatusNotFound, resp.status)
		})
	}, allure.Feature(featArticles), allure.Story(storyDelete),
		allure.Description("The author can delete their article; it is then unreadable"),
		allure.Severity(sevCritical))
}

func TestArticles_Delete_NonExistent(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")
		resp := ta.doReq(t, http.MethodDelete, pathArticle("never-existed"), nil, token)
		attach("response", resp)
		require.Equal(t, http.StatusNotFound, resp.status)
	}, allure.Feature(featArticles), allure.Story(storyDelete),
		allure.Description("Deleting a non-existent article returns 404"),
		allure.Severity(sevNormal))
}
