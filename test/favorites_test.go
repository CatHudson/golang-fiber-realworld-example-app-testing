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
		var created articleResp
		var reader string
		given("an article by an author and a separate reader", func() {
			author := ta.registerAndLogin(t, "author")
			reader = ta.registerAndLogin(t, "reader")
			created = ta.createArticle(t, author, "Fav Me")
		})

		var out articleResp
		var resp apiResp
		when("the reader POSTs /favorite", func() {
			resp = ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, reader)
			attach("response", resp)
		})
		then("the response is 200 OK", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("the write response shows favorited=true and favoritesCount=1", func() {
			require.True(t, out.Article.Favorited, "the favoriting user should see favorited=true in the write response")
			require.Equal(t, 1, out.Article.FavoritesCount)
		})
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("A user can favorite an article; the write response reflects favorited=true and count=1"),
		allure.Severity(sevCritical))
}

func TestFavorites_Favorite_Unauthenticated(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		given("an existing article", func() {
			author := ta.registerAndLogin(t, "author")
			created = ta.createArticle(t, author, "Fav Me")
		})

		var resp apiResp
		when("/favorite is POSTed with no token", func() {
			resp = ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, "")
			attach("response", resp)
		})
		then("the JWT middleware rejects it with 400", func() {
			require.Equal(t, http.StatusBadRequest, resp.status)
		})
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("Favoriting without a token is rejected by the JWT middleware (400)"),
		allure.Severity(sevNormal))
}

func TestFavorites_Favorite_NonExistentArticle(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var token string
		given("an authenticated user", func() {
			token = ta.registerAndLogin(t, "alice")
		})

		var resp apiResp
		when("she favorites a slug that does not exist", func() {
			resp = ta.doReq(t, http.MethodPost, pathFavorite("no-such-article"), nil, token)
			attach("response", resp)
		})
		then("the response is 404 Not Found", func() {
			require.Equal(t, http.StatusNotFound, resp.status)
		})
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("Favoriting a non-existent article returns 404"),
		allure.Severity(sevNormal))
}

func TestFavorites_CountAccumulatesAcrossUsers(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var r1, r2 string
		given("an article and two distinct readers", func() {
			author := ta.registerAndLogin(t, "author")
			r1 = ta.registerAndLogin(t, "reader1")
			r2 = ta.registerAndLogin(t, "reader2")
			created = ta.createArticle(t, author, "Popular")
		})
		given("the first reader has already favorited it", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, r1)
			require.Equal(t, http.StatusOK, resp.status)
		})

		var out articleResp
		var resp apiResp
		when("the second distinct reader favorites the same article", func() {
			resp = ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, r2)
			attach("response", resp)
		})
		then("the response is 200 OK", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("favoritesCount is 2 (favorites from distinct users accumulate)", func() {
			require.Equal(t, 2, out.Article.FavoritesCount)
		})
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("Favorites from distinct users accumulate in favoritesCount"),
		allure.Severity(sevNormal))
}

// ---- Unfavorite ----------------------------------------------------------

func TestFavorites_Unfavorite(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var reader string
		given("an article the reader has already favorited", func() {
			author := ta.registerAndLogin(t, "author")
			reader = ta.registerAndLogin(t, "reader")
			created = ta.createArticle(t, author, "Fav Toggle")
			ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, reader)
		})

		var out articleResp
		var resp apiResp
		when("the reader DELETEs /favorite to undo it", func() {
			resp = ta.doReq(t, http.MethodDelete, pathFavorite(created.Article.Slug), nil, reader)
			attach("response", resp)
		})
		then("the response is 200 OK", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("favoritesCount is back to 0 and favorited is false", func() {
			require.Equal(t, 0, out.Article.FavoritesCount)
			require.False(t, out.Article.Favorited)
		})
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
		var created articleResp
		var reader string
		given("an article and a reader", func() {
			author := ta.registerAndLogin(t, "author")
			reader = ta.registerAndLogin(t, "reader")
			created = ta.createArticle(t, author, "Read Fav Flag")
		})
		given("the reader has favorited the article", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, reader)
			require.Equal(t, http.StatusOK, resp.status)
		})

		var out articleResp
		var resp apiResp
		when("the reader re-reads the article with their token", func() {
			resp = ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, reader)
			attach("response", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		then("favoritesCount is correct (1)", func() {
			require.Equal(t, 1, out.Article.FavoritesCount)
		})
		and("favorited must be true for that reader — but BUG #3: GET skips JWT, so the token is ignored on reads and it stays false", func() {
			require.True(t, out.Article.Favorited,
				"BUG #3: favorited must be true for the user who favorited it, but GET skips JWT so the token is ignored on reads")
		})
	}, allure.Feature(featFavorites), allure.Story(storyFavorite),
		allure.Description("SPEC: a reader who favorited an article must see favorited=true on read. Documents bug #3 — runs RED."),
		allure.Tag(tagBug3), allure.Tag(tagSpec), allure.Severity(sevCritical))
}
