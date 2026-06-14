package test

// End-to-end user journeys. Where the per-feature files assert one endpoint at a
// time, these chain several endpoints into a single realistic story — publish →
// comment → edit → delete, follow → feed → unfollow, and so on — so the report
// shows the app working (or not) the way a real client would drive it.
//
// Every flow here exercises behaviour that is correct in the app, so they all run
// GREEN. They deliberately steer clear of the known defects (e.g. the favorited
// flag on read, bug #3; query-string filtering, bug #1), which are pinned down by
// the dedicated RED _SPEC tests in the per-feature files. The one place a bug
// borders a journey — re-reading a favourited article — the flow asserts only the
// favoritesCount (which is correct on read), not the favorited flag.

import (
	"net/http"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/stretchr/testify/require"
)

// TestFlow_Authoring walks one article through its whole life: an author publishes
// it, a reader finds it in the global list and comments, the author renames it
// (which regenerates the slug) and finally deletes it — after which it is gone for
// everyone.
func TestFlow_Authoring(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var author, reader string
		given("a registered author and a registered reader", func() {
			author = ta.registerAndLogin(t, "author")
			reader = ta.registerAndLogin(t, "reader")
		})

		var created articleResp
		when("the author publishes an article tagged go,testing", func() {
			created = ta.createArticle(t, author, "My First Post", "go", "testing")
		})
		then("it is publicly readable by slug, by the author, with its tags", func() {
			resp := ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, "")
			attach("read after publish", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var got articleResp
			decode(t, resp, &got)
			require.Equal(t, "author", got.Article.Author.Username)
			require.ElementsMatch(t, []string{"go", "testing"}, got.Article.TagList)
		})
		and("it shows up in the global article list", func() {
			list, resp := ta.listArticles(t, "")
			attach("global list", resp)
			require.Contains(t, list.slugs(), created.Article.Slug)
		})

		when("the reader comments on the article", func() {
			ta.addComment(t, reader, created.Article.Slug, "nice post")
		})
		then("the public comment list shows the reader's comment", func() {
			resp := ta.doReq(t, http.MethodGet, pathComments(created.Article.Slug), nil, "")
			attach("comments", resp)
			var out struct {
				Comments []struct {
					Body   string `json:"body"`
					Author struct {
						Username string `json:"username"`
					} `json:"author"`
				} `json:"comments"`
			}
			decode(t, resp, &out)
			require.Len(t, out.Comments, 1)
			require.Equal(t, "nice post", out.Comments[0].Body)
			require.Equal(t, "reader", out.Comments[0].Author.Username)
		})

		oldSlug := func() string { return created.Article.Slug }
		var newSlug string
		when("the author renames the article", func() {
			resp := ta.doReq(t, http.MethodPut, pathArticle(oldSlug()),
				map[string]any{"article": map[string]any{"title": "My Renamed Post"}}, author)
			attach("rename", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out articleResp
			decode(t, resp, &out)
			newSlug = out.Article.Slug
		})
		then("the slug is regenerated from the new title", func() {
			require.Equal(t, "my-renamed-post", newSlug)
		})
		and("the old slug no longer resolves while the new one does", func() {
			old := ta.doReq(t, http.MethodGet, pathArticle(oldSlug()), nil, "")
			require.Equal(t, http.StatusNotFound, old.status)
			cur := ta.doReq(t, http.MethodGet, pathArticle(newSlug), nil, "")
			require.Equal(t, http.StatusOK, cur.status)
		})

		when("the author deletes the article", func() {
			resp := ta.doReq(t, http.MethodDelete, pathArticle(newSlug), nil, author)
			attach("delete", resp)
			require.Equal(t, http.StatusOK, resp.status)
		})
		then("it is gone for everyone and the global list is empty again", func() {
			gone := ta.doReq(t, http.MethodGet, pathArticle(newSlug), nil, "")
			require.Equal(t, http.StatusNotFound, gone.status)
			list, _ := ta.listArticles(t, "")
			require.EqualValues(t, 0, list.ArticlesCount)
		})
	}, allure.Feature(featFlows), allure.Story(storyAuthoring),
		allure.Description("Author publishes → reader comments → author renames → author deletes; the article's whole life-cycle"),
		allure.Severity(sevCritical))
}

// TestFlow_FollowAndFeed exercises the social graph: a fan follows a celeb, the
// celeb's profile then reports following=true, the fan's personalised feed fills
// with the celeb's articles, and unfollowing empties the feed again.
func TestFlow_FollowAndFeed(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var celeb, fan string
		var first, second articleResp
		given("a celeb who has published two articles, and a fan", func() {
			celeb = ta.registerAndLogin(t, "celeb")
			fan = ta.registerAndLogin(t, "fan")
			first = ta.createArticle(t, celeb, "Celeb One")
			second = ta.createArticle(t, celeb, "Celeb Two")
		})

		given("the fan's feed starts empty (they follow no one yet)", func() {
			resp := ta.doReq(t, http.MethodGet, pathFeed, nil, fan)
			attach("empty feed", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out articleListResp
			decode(t, resp, &out)
			require.Empty(t, out.Articles)
		})

		when("the fan follows the celeb", func() {
			resp := ta.doReq(t, http.MethodPost, pathFollow("celeb"), nil, fan)
			attach("follow", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out profileResp
			decode(t, resp, &out)
			require.True(t, out.Profile.Following, "the follow response should report following=true")
		})
		then("reading the celeb's profile as the fan reports following=true", func() {
			resp := ta.doReq(t, http.MethodGet, pathProfile("celeb"), nil, fan)
			attach("profile", resp)
			var out profileResp
			decode(t, resp, &out)
			require.True(t, out.Profile.Following)
		})
		and("the fan's feed now holds both of the celeb's articles, newest first", func() {
			// NB: assert the article list itself — articlesCount is wrong here
			// (bug #6, see TestFlow_Feed_ArticlesCountMismatch_SPEC), so this
			// GREEN journey checks only what the feed gets right.
			resp := ta.doReq(t, http.MethodGet, pathFeed, nil, fan)
			attach("populated feed", resp)
			var out articleListResp
			decode(t, resp, &out)
			require.Equal(t, []string{second.Article.Slug, first.Article.Slug}, out.slugs())
		})

		when("the fan unfollows the celeb", func() {
			resp := ta.doReq(t, http.MethodDelete, pathFollow("celeb"), nil, fan)
			attach("unfollow", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out profileResp
			decode(t, resp, &out)
			require.False(t, out.Profile.Following)
		})
		then("the fan's feed is empty again", func() {
			resp := ta.doReq(t, http.MethodGet, pathFeed, nil, fan)
			attach("feed after unfollow", resp)
			var out articleListResp
			decode(t, resp, &out)
			require.Empty(t, out.Articles)
		})
	}, allure.Feature(featFlows), allure.Story(storySocial),
		allure.Description("Follow a user → their profile shows following → their posts fill your feed → unfollow empties it"),
		allure.Severity(sevCritical))
}

// TestFlow_Favoriting follows a popular article: it surfaces in the tag list, two
// readers favourite it (the count climbing to 2), and one then unfavourites it
// (back to 1). Counts are asserted on a fresh read — the favorited flag on read is
// bug #3 and is covered by its own RED _SPEC, so this flow leaves it alone.
func TestFlow_Favoriting(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var created articleResp
		var r1, r2 string
		given("an article tagged go and two readers", func() {
			author := ta.registerAndLogin(t, "author")
			r1 = ta.registerAndLogin(t, "reader1")
			r2 = ta.registerAndLogin(t, "reader2")
			created = ta.createArticle(t, author, "Popular Post", "go")
		})

		then("the tag appears in the global tag list", func() {
			resp := ta.doReq(t, http.MethodGet, pathTags, nil, "")
			attach("tags", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out tagsResp
			decode(t, resp, &out)
			require.Contains(t, out.Tags, "go")
		})

		when("the first reader favourites it", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, r1)
			attach("favorite r1", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out articleResp
			decode(t, resp, &out)
			require.True(t, out.Article.Favorited, "the favouriting reader sees favorited=true in the write response")
			require.Equal(t, 1, out.Article.FavoritesCount)
		})
		and("a second reader favourites it, pushing the count to 2", func() {
			resp := ta.doReq(t, http.MethodPost, pathFavorite(created.Article.Slug), nil, r2)
			attach("favorite r2", resp)
			var out articleResp
			decode(t, resp, &out)
			require.Equal(t, 2, out.Article.FavoritesCount)
		})

		when("the first reader unfavourites it", func() {
			resp := ta.doReq(t, http.MethodDelete, pathFavorite(created.Article.Slug), nil, r1)
			attach("unfavorite r1", resp)
			require.Equal(t, http.StatusOK, resp.status)
		})
		then("a fresh public read shows favoritesCount back at 1", func() {
			resp := ta.doReq(t, http.MethodGet, pathArticle(created.Article.Slug), nil, "")
			attach("read after unfavorite", resp)
			var out articleResp
			decode(t, resp, &out)
			require.Equal(t, 1, out.Article.FavoritesCount)
		})
	}, allure.Feature(featFlows), allure.Story(storyFavoriting),
		allure.Description("Article surfaces in tag list → two readers favourite (count 2) → one unfavourites (count 1)"),
		allure.Severity(sevNormal))
}

// TestFlow_Account covers a user's account life-cycle: register, log in with those
// credentials, read the current user, update the bio, and confirm the change
// persisted while the original credentials still work.
func TestFlow_Account(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var token string
		when("a new user registers", func() {
			token = ta.registerAndLogin(t, "bob")
		})
		then("logging in with those credentials returns a token", func() {
			resp := ta.login(t, "bob@realworld.io", "secret123")
			attach("login", resp)
			require.Equal(t, http.StatusOK, resp.status)
			var out userResp
			decode(t, resp, &out)
			require.NotEmpty(t, out.User.Token)
		})
		and("GET /api/user returns the current user with no bio yet", func() {
			resp := ta.doReq(t, http.MethodGet, pathUser, nil, token)
			attach("current user", resp)
			var out userResp
			decode(t, resp, &out)
			require.Equal(t, "bob", out.User.Username)
			require.Empty(t, deref(out.User.Bio))
		})

		when("the user updates their bio", func() {
			resp := ta.doReq(t, http.MethodPut, pathUser,
				map[string]any{"user": map[string]any{"bio": "I write Go"}}, token)
			attach("update", resp)
			require.Equal(t, http.StatusOK, resp.status)
		})
		then("a fresh read reflects the new bio and keeps the email", func() {
			resp := ta.doReq(t, http.MethodGet, pathUser, nil, token)
			attach("current user after update", resp)
			var out userResp
			decode(t, resp, &out)
			require.Equal(t, "I write Go", deref(out.User.Bio))
			require.Equal(t, "bob@realworld.io", out.User.Email)
		})
		and("the original credentials still authenticate", func() {
			resp := ta.login(t, "bob@realworld.io", "secret123")
			require.Equal(t, http.StatusOK, resp.status)
		})
	}, allure.Feature(featFlows), allure.Story(storyAccount),
		allure.Description("Register → login → read current user → update bio → bio persists and credentials still work"),
		allure.Severity(sevCritical))
}

// TestFlow_Feed_ArticlesCountMismatch_SPEC documents bug #6, found while building
// the follow→feed journey above. GET /api/articles/feed returns the right articles
// (the posts of everyone you follow) but the wrong articlesCount: the count query
// in store.ListFeed is
//
//	as.db.Where(&model.Article{AuthorID: u.ID}).Model(&model.Article{}).Count(&count)
//
// where u is the *feed viewer*, so it counts the viewer's OWN articles instead of
// the feed's. A reader who follows a prolific author but has written nothing sees
// a full list of articles alongside articlesCount:0 — a paginating client would
// conclude there are no results and stop. The count must equal the number of feed
// articles.
//
// FAILS until bug #6 is fixed (the feed count must reflect the feed, not the viewer).
func TestFlow_Feed_ArticlesCountMismatch_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var celeb, fan string
		given("a celeb with two articles, followed by a fan who has written nothing", func() {
			celeb = ta.registerAndLogin(t, "celeb")
			fan = ta.registerAndLogin(t, "fan")
			ta.createArticle(t, celeb, "Celeb One")
			ta.createArticle(t, celeb, "Celeb Two")
			resp := ta.doReq(t, http.MethodPost, pathFollow("celeb"), nil, fan)
			require.Equal(t, http.StatusOK, resp.status)
		})

		var out articleListResp
		when("the fan GETs their feed", func() {
			resp := ta.doReq(t, http.MethodGet, pathFeed, nil, fan)
			attach("feed", resp)
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		then("the feed lists both of the celeb's articles", func() {
			require.Len(t, out.Articles, 2)
		})
		and("articlesCount must equal 2 — but BUG #6: the count query counts the viewer's own articles, so it reports 0", func() {
			require.EqualValues(t, 2, out.ArticlesCount,
				"BUG #6: feed articlesCount counts the viewer's own articles (AuthorID = viewer) instead of the feed's, so it reports 0 while two articles are returned")
		})
	}, allure.Feature(featArticles), allure.Story(storyFeed),
		allure.Description("SPEC: the feed's articlesCount must match the feed. Documents bug #6 — runs RED."),
		allure.Tag(tagBug6), allure.Tag(tagSpec), allure.Severity(sevCritical))
}

// deref returns the pointed-to string, or "" for a nil pointer (bio/image are
// nullable in the user response).
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
