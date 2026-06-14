package test

import (
	"net/http"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/stretchr/testify/require"
)

// ---- Favorite ------------------------------------------------------------

func TestFavorites_Favorite_ByUser(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		author := ta.registerAndLogin(t, "author")
		reader := ta.registerAndLogin(t, "reader")
		created := ta.createArticle(t, author, "Fav Me")

		var out articleResp
		step("POST /favorite as a reader", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, reader)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		require.True(t, out.Article.Favorited, "the favoriting user should see favorited=true in the write response")
		require.Equal(t, 1, out.Article.FavoritesCount)
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("A user can favorite an article; the write response reflects favorited=true and count=1"),
		allure.Severity(sevCritical))
}

func TestFavorites_Favorite_Unauthenticated(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		author := ta.registerAndLogin(t, "author")
		created := ta.createArticle(t, author, "Fav Me")

		resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, "")
		attach("response", resp)
		require.Equal(t, http.StatusBadRequest, resp.status)
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("Favoriting without a token is rejected by the JWT middleware (400)"),
		allure.Severity(sevNormal))
}

func TestFavorites_Favorite_NonExistentArticle(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")
		resp := ta.doReq(t, http.MethodPost, pathFavorite("no-such-article"), nil, token)
		attach("response", resp)
		require.Equal(t, http.StatusNotFound, resp.status)
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("Favoriting a non-existent article returns 404"),
		allure.Severity(sevNormal))
}

func TestFavorites_CountAccumulatesAcrossUsers(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		author := ta.registerAndLogin(t, "author")
		r1 := ta.registerAndLogin(t, "reader1")
		r2 := ta.registerAndLogin(t, "reader2")
		created := ta.createArticle(t, author, "Popular")

		ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, r1)
		var out articleResp
		step("Second distinct user favorites the same article", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, r2)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		require.Equal(t, 2, out.Article.FavoritesCount, "two distinct users favoriting should count as 2")
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("Favorites from distinct users accumulate in favoritesCount"),
		allure.Severity(sevNormal))
}

// ---- Unfavorite ----------------------------------------------------------

func TestFavorites_Unfavorite(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		author := ta.registerAndLogin(t, "author")
		reader := ta.registerAndLogin(t, "reader")
		created := ta.createArticle(t, author, "Fav Toggle")

		ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, reader)

		var out articleResp
		step("DELETE /favorite to undo the favorite", func() {
			resp := ta.doReq(t, http.MethodDelete, pathFavorite(created.Article.Slug), nil, reader)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		require.Equal(t, 0, out.Article.FavoritesCount)
		require.False(t, out.Article.Favorited)
	}, allure.Feature(featFavorites), allure.Story(storyUnfavorite),
		allure.Description("Unfavoriting drops the count back to 0"),
		allure.Severity(sevCritical))
}

// TestFavorites_FavoritedFlagOnRead_SPEC documents bug #3: the `favorited` flag
// is always false when an article is *read*, even by the very user who favorited
// it. The articles route registers its JWT middleware with a filter that skips
// auth for every GET (handler/routes.go), so on a read the server never parses
// the caller's token and userIDFromToken(c) is 0 — newArticleResponse then can't
// match the current user against the Favorites list, so favorited is always false.
//
// favoritesCount is correct (the association is persisted), which makes the bug
// sneaky: the count says "1 person likes this" but the API can never tell *you*
// that the one person is you. The RealWorld contract requires favorited to
// reflect the authenticated reader.
//
// FAILS until bug #3 is fixed (reads must honour the token for the favorited flag).
func TestFavorites_FavoritedFlagOnRead_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		author := ta.registerAndLogin(t, "author")
		reader := ta.registerAndLogin(t, "reader")
		created := ta.createArticle(t, author, "Read Fav Flag")

		step("Reader favorites the article", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, reader)
			require.Equal(t, http.StatusOK, resp.status)
		})

		var out articleResp
		step("Reader re-reads the article with their token", func() {
			resp := ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, reader)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		// Count persists correctly...
		require.Equal(t, 1, out.Article.FavoritesCount)
		// ...but favorited is wrong because GET ignores the token. This is the bug.
		require.True(t, out.Article.Favorited,
			"BUG #3: favorited must be true for the user who favorited it, but GET skips JWT so the token is ignored on reads")
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("SPEC: a reader who favorited an article must see favorited=true on read. Documents bug #3 — runs RED."),
		allure.Tag(tagBug3), allure.Tag(tagSpec), allure.Severity(sevCritical))
}
