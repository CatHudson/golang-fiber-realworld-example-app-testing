package test

import (
	"net/http"
	"testing"

	"github.com/dailymotion/allure-go"
	"github.com/dailymotion/allure-go/severity"
	"github.com/stretchr/testify/require"
)

// TestSmoke_RegisterUser is the pipeline's first passing test: it proves the
// harness wires a real app end-to-end and that Allure step/attachment reporting
// emits results. Intentionally trivial — the interesting (and red) tests arrive
// in later commits. An early near-empty report is deliberate: it shows the CI
// pipeline predates the suite.
func TestSmoke_RegisterUser(t *testing.T) {
	allure.Test(t,
		allure.Description("Registering a new user returns 201 with a token (smoke test for the harness + Allure wiring)"),
		allure.Action(func() {
			ta := newApp(t)

			var resp apiResp
			var out userResp
			allure.Step(
				allure.Description("POST /api/users with a fresh account"),
				allure.Action(func() {
					out, resp = ta.register(t, "smoke_alice")
					_ = allure.AddAttachment("response", allure.ApplicationJson, resp.body)
				}),
			)

			allure.Step(
				allure.Description("Assert 201 Created and a non-empty token + echoed fields"),
				allure.Action(func() {
					require.Equal(t, http.StatusCreated, resp.status)
					require.Equal(t, "smoke_alice", out.User.Username)
					require.Equal(t, "smoke_alice@realworld.io", out.User.Email)
					require.NotEmpty(t, out.User.Token)
				}),
			)
		}),
		allure.Epic("API"),
		allure.Feature("Smoke"),
		allure.Severity(severity.Blocker),
	)
}
