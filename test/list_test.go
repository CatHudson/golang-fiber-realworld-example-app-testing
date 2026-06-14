package test

import (
	"net/http"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/stretchr/testify/require"
)

// ---- Baseline (works) ----------------------------------------------------

func TestArticles_List_ReturnsAllNewestFirst(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("three articles published in order: First, Second, Third", func() {
			token := ta.registerAndLogin(t, "alice")
			ta.createArticle(t, token, "First")
			ta.createArticle(t, token, "Second")
			ta.createArticle(t, token, "Third")
		})

		var out articleListResp
		when("GET /api/articles with no query", func() {
			var resp apiResp
			out, resp = ta.listArticles(t, "")
			attach("response", resp)
		})
		then("all three are returned", func() {
			require.EqualValues(t, 3, out.ArticlesCount)
		})
		and("they are ordered newest first", func() {
			require.Equal(t, []string{"third", "second", "first"}, out.slugs())
		})
	}, allure.Feature(featArticles), allure.Story(storyFilter),
		allure.Description("Listing articles returns them all, newest first"),
		allure.Severity(sevNormal))
}

// ---- Bug #1: query params are read with c.Params (route params) instead of
// c.Query (query string), so every filter and the limit/offset pagination are
// silently ignored. The list always returns all articles with the default
// offset=0, limit=20. Each _SPEC test asserts the RealWorld contract and runs
// RED until the handler switches to c.Query.

func TestArticles_List_FilterByTag_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("two 'go' articles and one 'rust' article", func() {
			token := ta.registerAndLogin(t, "alice")
			ta.createArticle(t, token, "Go One", "go")
			ta.createArticle(t, token, "Rust One", "rust")
			ta.createArticle(t, token, "Go Two", "go")
		})

		var out articleListResp
		when("GET /api/articles?tag=rust", func() {
			var resp apiResp
			out, resp = ta.listArticles(t, "?tag=rust")
			attach("response", resp)
		})
		then("only the rust article is returned — but BUG #1: ?tag is read via c.Params, so the filter is ignored and all 3 come back", func() {
			require.EqualValues(t, 1, out.ArticlesCount,
				"BUG #1: ?tag should filter to matching articles; handler reads c.Params(\"tag\") so the filter is ignored")
			require.Equal(t, []string{"rust-one"}, out.slugs())
		})
	}, allure.Feature(featArticles), allure.Story(storyFilter),
		allure.Description("SPEC: ?tag must filter by tag. Documents bug #1 — runs RED."),
		allure.Tag(tagBug1), allure.Tag(tagSpec), allure.Severity(sevCritical))
}

func TestArticles_List_FilterByAuthor_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("two articles by alice and one by bob", func() {
			alice := ta.registerAndLogin(t, "alice")
			bob := ta.registerAndLogin(t, "bob")
			ta.createArticle(t, alice, "Alice One")
			ta.createArticle(t, alice, "Alice Two")
			ta.createArticle(t, bob, "Bob One")
		})

		var out articleListResp
		when("GET /api/articles?author=bob", func() {
			var resp apiResp
			out, resp = ta.listArticles(t, "?author=bob")
			attach("response", resp)
		})
		then("only bob's article is returned — but BUG #1: ?author is read via c.Params, so the filter is ignored and all 3 come back", func() {
			require.EqualValues(t, 1, out.ArticlesCount,
				"BUG #1: ?author should filter by author; handler reads c.Params(\"author\") so the filter is ignored")
			require.Equal(t, []string{"bob-one"}, out.slugs())
		})
	}, allure.Feature(featArticles), allure.Story(storyFilter),
		allure.Description("SPEC: ?author must filter by author. Documents bug #1 — runs RED."),
		allure.Tag(tagBug1), allure.Tag(tagSpec), allure.Severity(sevCritical))
}

func TestArticles_List_FilterByFavorited_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var liked articleResp
		var reader string
		given("two articles, one of which the reader favorites", func() {
			author := ta.registerAndLogin(t, "author")
			reader = ta.registerAndLogin(t, "reader")
			liked = ta.createArticle(t, author, "Liked")
			ta.createArticle(t, author, "Ignored")
			resp := ta.doReq(t, http.MethodPost, pathFavorite(liked.Article.Slug), nil, reader)
			require.Equal(t, http.StatusOK, resp.status)
		})

		var out articleListResp
		when("GET /api/articles?favorited=reader", func() {
			var resp apiResp
			out, resp = ta.listArticles(t, "?favorited=reader")
			attach("response", resp)
		})
		then("only the favorited article is returned — but BUG #1: ?favorited is read via c.Params, so the filter is ignored and both come back", func() {
			require.EqualValues(t, 1, out.ArticlesCount,
				"BUG #1: ?favorited should return only what the user favorited; handler reads c.Params(\"favorited\") so the filter is ignored")
			require.Equal(t, []string{"liked"}, out.slugs())
		})
	}, allure.Feature(featArticles), allure.Story(storyFilter),
		allure.Description("SPEC: ?favorited must filter to a user's favorites. Documents bug #1 — runs RED."),
		allure.Tag(tagBug1), allure.Tag(tagSpec), allure.Severity(sevNormal))
}

func TestArticles_List_Limit_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("three articles", func() {
			token := ta.registerAndLogin(t, "alice")
			ta.createArticle(t, token, "One")
			ta.createArticle(t, token, "Two")
			ta.createArticle(t, token, "Three")
		})

		var out articleListResp
		when("GET /api/articles?limit=1", func() {
			var resp apiResp
			out, resp = ta.listArticles(t, "?limit=1")
			attach("response", resp)
		})
		then("exactly one article is returned — but BUG #1: ?limit is read via c.Params, so it always defaults to 20", func() {
			require.Len(t, out.Articles, 1,
				"BUG #1: ?limit should cap the page size; handler reads c.Params(\"limit\") so it always defaults to 20")
		})
	}, allure.Feature(featArticles), allure.Story(storyFilter),
		allure.Description("SPEC: ?limit must cap page size. Documents bug #1 — runs RED."),
		allure.Tag(tagBug1), allure.Tag(tagSpec), allure.Severity(sevNormal))
}

func TestArticles_List_Pagination_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("three articles (newest-first order is Three, Two, One)", func() {
			token := ta.registerAndLogin(t, "alice")
			ta.createArticle(t, token, "One")
			ta.createArticle(t, token, "Two")
			ta.createArticle(t, token, "Three")
		})

		var out articleListResp
		when("GET /api/articles?limit=1&offset=1 (second page of size 1)", func() {
			var resp apiResp
			out, resp = ta.listArticles(t, "?limit=1&offset=1")
			attach("response", resp)
		})
		then("just the second-newest ('two') is returned — but BUG #1: limit/offset are read via c.Params, so pagination is ignored", func() {
			require.Len(t, out.Articles, 1,
				"BUG #1: limit/offset are read via c.Params so pagination is ignored — the full list is returned")
			require.Equal(t, []string{"two"}, out.slugs())
		})
	}, allure.Feature(featArticles), allure.Story(storyFilter),
		allure.Description("SPEC: limit+offset must page through results. Documents bug #1 — runs RED."),
		allure.Tag(tagBug1), allure.Tag(tagSpec), allure.Severity(sevNormal))
}
