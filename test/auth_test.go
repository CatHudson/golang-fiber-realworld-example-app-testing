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
		out, resp := ta.register(t, "alice")
		attach("response", resp)
		require.Equal(t, http.StatusCreated, resp.status)
		require.Equal(t, "alice", out.User.Username)
		require.Equal(t, "alice@realworld.io", out.User.Email)
		require.NotEmpty(t, out.User.Token, "registration must return a JWT")
	}, allure.Feature(featAuth), allure.Story(storyRegistration),
		allure.Description("Registering a new user returns 201 with the user and a JWT"),
		allure.Severity(sevCritical))
}

func TestAuth_Register_DuplicateEmail(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		_, first := ta.register(t, "alice")
		require.Equal(t, http.StatusCreated, first.status)

		// Same email (register uses "<username>@realworld.io"); reuse the email
		// with a different username to isolate the unique-email constraint.
		var resp apiResp
		step("Register a second user with the same email", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "alice2",
				"email":    "alice@realworld.io",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		require.Equal(t, http.StatusUnprocessableEntity, resp.status,
			"duplicate email must be rejected, got %d: %s", resp.status, string(resp.body))
	}, allure.Feature(featAuth), allure.Story(storyRegistration),
		allure.Description("Registering a duplicate email is rejected with 422"),
		allure.Severity(sevNormal))
}

func TestAuth_Register_MissingUsername(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		resp := ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
			"email":    "nouser@realworld.io",
			"password": "secret123",
		}}, "")
		attach("response", resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.status)
	}, allure.Feature(featAuth), allure.Story(storyValidation),
		allure.Description("Missing username fails validation with 422 (username's validate tag is well-formed)"),
		allure.Severity(sevNormal))
}

func TestAuth_Register_MissingPassword(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		resp := ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
			"username": "nopass",
			"email":    "nopass@realworld.io",
		}}, "")
		attach("response", resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.status)
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
		step("Register with no email field at all", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "noemail",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		require.Equal(t, http.StatusUnprocessableEntity, resp.status,
			"BUG #5: missing email should be 422; malformed struct tag disables the `required` rule (got %d)", resp.status)
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
		step("Register with a malformed email address", func() {
			resp = ta.doReq(t, http.MethodPost, pathUsers, map[string]any{"user": map[string]string{
				"username": "bademail",
				"email":    "not-an-email",
				"password": "secret123",
			}}, "")
			attach("response", resp)
		})
		require.Equal(t, http.StatusUnprocessableEntity, resp.status,
			"BUG #5: invalid email should be 422; `email` validator never runs (got %d)", resp.status)
	}, allure.Feature(featAuth), allure.Story(storyValidation),
		allure.Description("SPEC: an invalid email format must be rejected (422). Documents bug #5 — runs RED."),
		allure.Tag(tagBug5), allure.Tag(tagSpec), allure.Severity(sevCritical))
}

// ---- Login ---------------------------------------------------------------

func TestAuth_Login_Success(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		ta.registerAndLogin(t, "alice")

		var resp apiResp
		var out userResp
		step("POST /api/users/login with correct credentials", func() {
			resp = ta.login(t, "alice@realworld.io", "secret123")
			attach("response", resp)
		})
		require.Equal(t, http.StatusOK, resp.status)
		decode(t, resp, &out)
		require.Equal(t, "alice", out.User.Username)
		require.NotEmpty(t, out.User.Token)
	}, allure.Feature(featAuth), allure.Story(storyLogin),
		allure.Description("Logging in with valid credentials returns 200 and a token"),
		allure.Severity(sevCritical))
}

func TestAuth_Login_WrongPassword(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		ta.registerAndLogin(t, "alice")
		resp := ta.login(t, "alice@realworld.io", "wrong-password")
		attach("response", resp)
		require.Equal(t, http.StatusForbidden, resp.status)
	}, allure.Feature(featAuth), allure.Story(storyLogin),
		allure.Description("Logging in with a wrong password is rejected with 403"),
		allure.Severity(sevNormal))
}

func TestAuth_Login_UnknownUser(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		resp := ta.login(t, "ghost@realworld.io", "secret123")
		attach("response", resp)
		require.Equal(t, http.StatusForbidden, resp.status)
	}, allure.Feature(featAuth), allure.Story(storyLogin),
		allure.Description("Logging in as an unknown user is rejected with 403"),
		allure.Severity(sevNormal))
}

// ---- Current user --------------------------------------------------------

func TestAuth_CurrentUser_WithToken(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		token := ta.registerAndLogin(t, "alice")
		resp := ta.doReq(t, http.MethodGet, pathUser, nil, token)
		attach("response", resp)
		require.Equal(t, http.StatusOK, resp.status)
		var out userResp
		decode(t, resp, &out)
		require.Equal(t, "alice", out.User.Username)
	}, allure.Feature(featAuth), allure.Story(storyCurrentUser),
		allure.Description("GET /api/user with a valid token returns 200 and the current user"),
		allure.Severity(sevCritical))
}

func TestAuth_CurrentUser_NoToken(t *testing.T) {
	t.Parallel()
	runTest(t, func() {
		ta := newApp(t)
		resp := ta.doReq(t, http.MethodGet, pathUser, nil, "")
		attach("response", resp)
		// jwtware rejects a missing token with 400 "missing or malformed JWT".
		// (Spec-purists would prefer 401, but this is middleware default
		// behaviour, not an app bug — we assert the actual contract.)
		require.Equal(t, http.StatusBadRequest, resp.status)
	}, allure.Feature(featAuth), allure.Story(storyCurrentUser),
		allure.Description("GET /api/user without a token is rejected by the JWT middleware (400)"),
		allure.Severity(sevNormal))
}
