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
		var token string
		given("an authenticated author alice", func() {
			token = ta.registerAndLogin(t, "alice")
		})

		var out articleResp
		var resp apiResp
		when("she POSTs a new article to /api/articles", func() {
			resp = ta.doReq(t, http.MethodPost, pathArticles, map[string]any{"article": map[string]any{
				"title":       "How to Train Your Dragon",
				"description": "ever wonder how?",
				"body":        "very carefully.",
				"tagList":     []string{"dragons", "training"},
			}}, token)
			attach("response", resp)
		})
		then("the response is 201 Created", func() {
			require.Equal(t, http.StatusCreated, resp.status)
			decode(t, resp, &out)
		})
		and("the slug is derived from the title", func() {
			require.Equal(t, "how-to-train-your-dragon", out.Article.Slug)
		})
		and("the author and tags are echoed back", func() {
			require.Equal(t, "alice", out.Article.Author.Username)
			require.ElementsMatch(t, []string{"dragons", "training"}, out.Article.TagList)
		})
	}, allure.Feature(featArticles), allure.Story(storyCreate),
		allure.Description("An authenticated user can create an article; the slug is derived from the title"),
		allure.Severity(sevCritical))
}

func TestArticles_Create_Unauthenticated(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("POSTing an article with no token", func() {
			resp = ta.doReq(t, http.MethodPost, pathArticles, map[string]any{"article": map[string]any{
				"title": "No Auth", "description": "d", "body": "b",
			}}, "")
			attach("response", resp)
		})
		then("the JWT middleware rejects it with 400", func() {
			require.Equal(t, http.StatusBadRequest, resp.status)
		})
	}, allure.Feature(featArticles), allure.Story(storyCreate),
		allure.Description("Creating an article without a token is rejected by the JWT middleware"),
		allure.Severity(sevNormal))
}

func TestArticles_GetBySlug_Public(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		given("an article published by alice", func() {
			token := ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Public Read")
		})

		var out articleResp
		var resp apiResp
		when("anyone GETs the article by slug with no auth", func() {
			resp = ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, "")
			attach("response", resp)
		})
		then("the response is 200 OK (reads are public)", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("it returns the requested article", func() {
			require.Equal(t, created.Article.Slug, out.Article.Slug)
			require.Equal(t, "alice", out.Article.Author.Username)
		})
	}, allure.Feature(featArticles), allure.Story(storyRead),
		allure.Description("Reading an article by slug is public and returns 200"),
		allure.Severity(sevNormal))
}

func TestArticles_GetBySlug_NotFound(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("GETting a slug that does not exist", func() {
			resp = ta.doReq(t, http.MethodGet, pathArticle("no-such-article"), nil, "")
			attach("response", resp)
		})
		then("the response is 404 Not Found", func() {
			require.Equal(t, http.StatusNotFound, resp.status)
		})
	}, allure.Feature(featArticles), allure.Story(storyRead),
		allure.Description("Reading a non-existent slug returns 404"),
		allure.Severity(sevNormal))
}

func TestArticles_Update_ByAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var token string
		given("an article published by alice", func() {
			token = ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Original Title")
		})

		var out articleResp
		var resp apiResp
		when("the author PUTs a new title and body", func() {
			resp = ta.doReq(t, http.MethodPut, pathArticle(created.Article.Slug), map[string]any{"article": map[string]any{
				"title": "Updated Title",
				"body":  "updated body",
			}}, token)
			attach("response", resp)
		})
		then("the response is 200 OK", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("the title is updated and the slug is regenerated from it", func() {
			require.Equal(t, "Updated Title", out.Article.Title)
			require.Equal(t, "updated-title", out.Article.Slug)
		})
	}, allure.Feature(featArticles), allure.Story(storyUpdate),
		allure.Description("The author can update their article; the slug is regenerated"),
		allure.Severity(sevCritical))
}

func TestArticles_Update_ByNonAuthor(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var bob string
		given("an article by alice and a second user bob", func() {
			alice := ta.registerAndLogin(t, "alice")
			bob = ta.registerAndLogin(t, "bob")
			created = ta.createArticle(t, alice, "Alice's Article")
		})

		var resp apiResp
		when("bob tries to PUT alice's article", func() {
			resp = ta.doReq(t, http.MethodPut, pathArticle(created.Article.Slug), map[string]any{"article": map[string]any{
				"title": "Hijacked",
			}}, bob)
			attach("response", resp)
		})
		then("the write is prevented with 404 (the store filters by author id, so the row is invisible to bob)", func() {
			require.Equal(t, http.StatusNotFound, resp.status)
		})
		and("alice's article is left unchanged", func() {
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
		var created articleResp
		var token string
		given("an article published by alice", func() {
			token = ta.registerAndLogin(t, "alice")
			created = ta.createArticle(t, token, "Delete Me")
		})

		when("the author DELETEs it", func() {
			resp := ta.doReq(t, http.MethodDelete, pathArticle(created.Article.Slug), nil, token)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
		})
		then("the article is gone (a later read returns 404)", func() {
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
		var token string
		given("an authenticated user with no such article", func() {
			token = ta.registerAndLogin(t, "alice")
		})

		var resp apiResp
		when("she DELETEs a slug that never existed", func() {
			resp = ta.doReq(t, http.MethodDelete, pathArticle("never-existed"), nil, token)
			attach("response", resp)
		})
		then("the response is 404 Not Found", func() {
			require.Equal(t, http.StatusNotFound, resp.status)
		})
	}, allure.Feature(featArticles), allure.Story(storyDelete),
		allure.Description("Deleting a non-existent article returns 404"),
		allure.Severity(sevNormal))
}
