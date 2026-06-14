package test

import (
	"net/http"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/stretchr/testify/require"
)

// ---- Registration --------------------------------------------------------

func TestAuth_Register_Success(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var out userResp
		var resp apiResp
		when("a new user registers via POST /api/users", func() {
			out, resp = ta.register(t, "alice")
			attach("response", resp)
		})
		then("the response is 201 Created", func() {
			require.Equal(t, http.StatusCreated, resp.status)
		})
		and("the body echoes the username and email", func() {
			require.Equal(t, "alice", out.User.Username)
			require.Equal(t, "alice@realworld.io", out.User.Email)
		})
		and("a JWT is returned for the new user", func() {
			require.NotEmpty(t, out.User.Token, "registration must return a JWT")
		})
	}, allure.Feature(featAuth), allure.Story(storyRegistration),
		allure.Description("Registering a new user returns 201 with the user and a JWT"),
		allure.Severity(sevCritical))
}

func TestAuth_Register_DuplicateEmail(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("a user already registered with alice@realworld.io", func() {
			_, first := ta.register(t, "alice")
			require.Equal(t, http.StatusCreated, first.status)
		})

		var resp apiResp
		when("a second user registers with the same email (different username)", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "alice2",
				"email":    "alice@realworld.io",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		then("the duplicate email is rejected with 422", func() {
			require.Equal(t, http.StatusUnprocessableEntity, resp.status,
				"duplicate email must be rejected, got %d: %s", resp.status, string(resp.body))
		})
	}, allure.Feature(featAuth), allure.Story(storyRegistration),
		allure.Description("Registering a duplicate email is rejected with 422"),
		allure.Severity(sevNormal))
}

func TestAuth_Register_MissingUsername(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("registering with no username field", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"email":    "nouser@realworld.io",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		then("validation rejects it with 422", func() {
			require.Equal(t, http.StatusUnprocessableEntity, resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyValidation),
		allure.Description("Missing username fails validation with 422 (username's validate tag is well-formed)"),
		allure.Severity(sevNormal))
}

func TestAuth_Register_MissingPassword(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("registering with no password field", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "nopass",
				"email":    "nopass@realworld.io",
			}}, "")
			attach("response", resp)
		})
		then("validation rejects it with 422", func() {
			require.Equal(t, http.StatusUnprocessableEntity, resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyValidation),
		allure.Description("Missing password fails validation with 422"),
		allure.Severity(sevNormal))
}

// TestAuth_Register_MissingEmail_SPEC documents bug #5: the Email field's struct
// tag in handler/request.go is malformed — `json: "email"` (space after the
// colon) breaks tag parsing for the whole field, which silently disables its
// `validate:"required, email"` rule too. So a registration with NO email is
// accepted (201) when the RealWorld contract requires a 422.
//
// FAILS until bug #5 is fixed.
func TestAuth_Register_MissingEmail_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("registering with no email field at all", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "noemail",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		then("it must be rejected with 422 (BUG #5: a malformed struct tag disables the `required` rule, so it returns 201)", func() {
			require.Equal(t, http.StatusUnprocessableEntity, resp.status,
				"BUG #5: missing email should be 422; malformed struct tag disables the `required` rule (got %d)", resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyValidation),
		allure.Description("SPEC: registering without an email must be rejected (422). Documents bug #5 — runs RED."),
		allure.Tag(tagBug5), allure.Tag(tagSpec), allure.Severity(sevCritical))
}

// TestAuth_Register_InvalidEmail_SPEC documents bug #5 from the other angle: a
// syntactically invalid email ("not-an-email") is accepted (201) because the
// `email` format validator never runs on the field. Spec wants 422.
//
// FAILS until bug #5 is fixed.
func TestAuth_Register_InvalidEmail_SPEC(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("registering with a malformed email address", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "bademail",
				"email":    "not-an-email",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		then("it must be rejected with 422 (BUG #5: the `email` validator never runs, so it returns 201)", func() {
			require.Equal(t, http.StatusUnprocessableEntity, resp.status,
				"BUG #5: invalid email should be 422; `email` validator never runs (got %d)", resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyValidation),
		allure.Description("SPEC: an invalid email format must be rejected (422). Documents bug #5 — runs RED."),
		allure.Tag(tagBug5), allure.Tag(tagSpec), allure.Severity(sevCritical))
}

// ---- Login ---------------------------------------------------------------

func TestAuth_Login_Success(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("a registered user alice", func() {
			ta.registerAndLogin(t, "alice")
		})

		var resp apiResp
		var out userResp
		when("she POSTs valid credentials to /api/users/login", func() {
			resp = ta.login(t, "alice@realworld.io", "secret123")
			attach("response", resp)
		})
		then("the response is 200 OK", func() {
			require.Equal(t, http.StatusOK, resp.status)
			decode(t, resp, &out)
		})
		and("it returns her username and a token", func() {
			require.Equal(t, "alice", out.User.Username)
			require.NotEmpty(t, out.User.Token)
		})
	}, allure.Feature(featAuth), allure.Story(storyLogin),
		allure.Description("Logging in with valid credentials returns 200 and a token"),
		allure.Severity(sevCritical))
}

func TestAuth_Login_WrongPassword(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		given("a registered user alice", func() {
			ta.registerAndLogin(t, "alice")
		})

		var resp apiResp
		when("she logs in with the wrong password", func() {
			resp = ta.login(t, "alice@realworld.io", "wrong-password")
			attach("response", resp)
		})
		then("the login is rejected with 403", func() {
			require.Equal(t, http.StatusForbidden, resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyLogin),
		allure.Description("Logging in with a wrong password is rejected with 403"),
		allure.Severity(sevNormal))
}

func TestAuth_Login_UnknownUser(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("logging in as a user who was never registered", func() {
			resp = ta.login(t, "ghost@realworld.io", "secret123")
			attach("response", resp)
		})
		then("the login is rejected with 403", func() {
			require.Equal(t, http.StatusForbidden, resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyLogin),
		allure.Description("Logging in as an unknown user is rejected with 403"),
		allure.Severity(sevNormal))
}

// ---- Current user --------------------------------------------------------

func TestAuth_CurrentUser_WithToken(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		var token string
		given("a registered, authenticated user alice", func() {
			token = ta.registerAndLogin(t, "alice")
		})

		var resp apiResp
		when("she requests GET /api/user with her token", func() {
			resp = ta.doReq(t, http.MethodGet, pathUser, nil, token)
			attach("response", resp)
		})
		then("the response is 200 OK with the current user", func() {
			require.Equal(t, http.StatusOK, resp.status)
			var out userResp
			decode(t, resp, &out)
			require.Equal(t, "alice", out.User.Username)
		})
	}, allure.Feature(featAuth), allure.Story(storyCurrentUser),
		allure.Description("GET /api/user with a valid token returns 200 and the current user"),
		allure.Severity(sevCritical))
}

func TestAuth_CurrentUser_NoToken(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)

		var resp apiResp
		when("requesting GET /api/user with no token", func() {
			resp = ta.doReq(t, http.MethodGet, pathUser, nil, "")
			attach("response", resp)
		})
		then("the JWT middleware rejects it with 400 (spec-purists would prefer 401; this is jwtware's default)", func() {
			require.Equal(t, http.StatusBadRequest, resp.status)
		})
	}, allure.Feature(featAuth), allure.Story(storyCurrentUser),
		allure.Description("GET /api/user without a token is rejected by the JWT middleware (400)"),
		allure.Severity(sevNormal))
}
