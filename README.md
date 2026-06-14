# SDET assignment — testing notes

> This is a fork of the [alpody/golang-fiber-realworld-example-app](https://github.com/alpody/golang-fiber-realworld-example-app) Conduit backend, with an automated test suite added on top. Everything below the `─────` separator is the upstream project's original README, left untouched.
>
> - **Fork:** https://github.com/CatHudson/golang-fiber-realworld-example-app-testing
> - **Live Allure report (CI-published):** https://cathudson.github.io/golang-fiber-realworld-example-app-testing/17/# You can change the number in the link to see the test history.

## The app I picked, and why

A Go/Fiber/GORM/SQLite implementation of Conduit. I wanted a backend in a stack I'm comfortable with, small enough to hold in my head, and with effectively no meaningful test coverage to build on — this one fit: a handful of handlers, an in-memory SQLite option that makes the suite hermetic and fast, and it runs without a fight.

## What I tested (and what I left out)

The suite is **service-level**: it boots the real app in-process (real router, JWT middleware, GORM, fresh in-memory SQLite per test) and drives it over HTTP. Nothing is mocked, so real behaviour — and the bugs — surface.

- **Core API + negatives:** auth (register/login/current-user, dup email, missing/invalid fields, wrong password, no token), articles CRUD (incl. non-author 404, public reads), comments, favorites, filtering & pagination.
- **4 end-to-end flows:** authoring lifecycle (publish → read → comment → rename → delete), follow → feed → unfollow, favoriting across users, and account lifecycle (register → login → read → update → re-login).
- **One unit test** where a real seam exists: `utils/jwt` round-trip (multiple ids, expiry, wrong-secret/alg-substitution guard).
- **Left out, on purpose:** the slug helper (it's a one-line call into a third-party lib — testing it tests someone else's code), an exhaustive validation matrix, and the frontend (there isn't one). I also **kept upstream's `handler/*_test.go` as-is** — they're the baseline; my `test/` suite supersedes them at the service level with richer assertions, per-test isolation, and Allure.

**49 tests total** (46 service + 3 unit). **11 of them are intentionally RED** — see below.

## Bugs I found (the interesting part)

I treat the app as the source of truth for *what it does*, but the RealWorld spec as the source of truth for *what's correct*. Where they disagree, I **did not fix the app** — I wrote a test asserting the correct behaviour, tagged it, and let it run RED. CI uses `|| true` so the report always publishes; the red tests are the findings.

| # | Where | Bug | RED tests |
|---|-------|-----|-----------|
| 1 | `handler/article.go` → `Articles` | Filtering & pagination silently ignored — `?tag/author/favorited/limit/offset` read via `c.Params` (route params) instead of `c.Query`. One root cause, five broken features. | 5 |
| 2 | `handler/article.go` → `DeleteComment` | A datastore error calls `log.Fatal` → `os.Exit`, killing the whole server process instead of returning 500. Latent DoS. | 1 (forked-subprocess test) |
| 3 | `handler/routes.go` | `favorited` flag always `false` on reads — the articles JWT filter skips auth on every GET, so the token is never parsed and the reader can't be matched. | 1 |
| 4 | `handler/request.go` | Malformed struct tag (`json: "email"`, space after colon) breaks tag parsing for the field, silently disabling its `required`+`email` validation. Missing/invalid emails are accepted. | 2 |
| 5 | `store/article.go` → `ListFeed` | Feed returns the right articles but the wrong `articlesCount` — the count query counts the *viewer's own* articles, not the feed's. A paginating client sees a full page next to `count:0`. **Found while writing the feed flow.** | 1 |
| 6 | `handler/request.go` → `userLoginRequest` | A **sibling of bug #4, same root cause, different field:** the login password carries `validate: "required"` (space after the colon), so `StructTag` parsing fails and `required` never runs. A login with no password skips validation and falls through to the bcrypt check, returning **403 instead of a 422 validation error**. | 1 |

> The plan started with five candidate bugs; one didn't pan out under probing, and the `ListFeed` count bug (#5) turned up empirically while writing the feed flow instead. Net: five confirmed.
>
> **#6 was added after a second pass.** Diagnosing bug #4 as "a space in a struct tag" implies a *class*, so I grepped the rest of `request.go` for the same shape — and the login password's `validate:` tag is broken the same way. That's bug #6. (One more, cosmetic: `json:"tagList, omitempty"` has a stray space, so `omitempty` silently never applies — inbound binding is unaffected, so I noted it but didn't write a test.) The real lesson: when a bug is a *pattern*, sweep for the pattern.

The recurring method: **probe before asserting.** For each suspected bug I ran a throwaway request against the real endpoint and read the raw response before committing to an assertion or an explanation — which repeatedly mattered (see bug #3 below).

## How I worked with the agent

**Tool:** Claude Code (Opus). First, we wrote a plan to follow later in iterations. I drove it in commit-sized batches — it proposed, I reviewed each diff and the proposed commit message, and I did all the committing/merging myself.

**Where it helped:** scaffolding the in-process harness, root-causing bugs via probes, and large mechanical refactors — fast and consistent once pointed in the right direction.

**Where it was wrong / I had to correct it:**
- It mislabelled **bug #3** from the plan as a "stale read-after-write" count bug. The probe showed the count is actually correct and the real defect is the `favorited` flag on read. Asserting the plan's story would have shipped a test that passes and misses the bug.
- For the clean-checkout build failure it first wanted to **install the swagger toolchain in CI**, then to **filter the `main` package out of `go test`**. I rejected both — the real fix was commenting out one unused `docs` import, so CI stays plain `go test ./...`.
- It flagged upstream's handler tests as duplication and wanted to **delete them**; I overrode that (they're upstream's baseline, not our mess — provenance note, not deletion).
- Its tests asserted at the request level ("sent request X, got 200") and made no use of Allure's structure. I insisted it restructure every test into **human-readable Given/When/Then steps** with assertions phrased as expectations, so the report reads like a spec, not a request log.

**The direction I'm happiest with — CI and the report first.** Before writing a single real test I had the agent stand up GitHub Actions + Allure *with history publishing to Pages*. So every commit since then accretes run-over-run history, and the report is the living artifact of the work — not a screenshot bolted on at the end.

That decision drove a clean **override.** The agent's design put the suite behind a `//go:build integration` tag for an "opt-in slow DB suite." I killed it: the suite is in-memory and runs in milliseconds, so a suite that *can* be skipped in CI protects nothing — all tests run in CI, or what's the point. I also pushed it on craft it wouldn't have volunteered — e.g. extracting the inlined API paths and Allure labels into constants.

## Running it / CI

```bash
go test ./...          # whole suite; ~1s, in-memory SQLite, no setup
# Allure: results auto-write to ./allure-results locally; CI publishes to GitHub Pages.
```

CI (`.github/workflows/ci.yml`) runs on PRs, emits Allure results, merges history from the `gh-pages` branch, and publishes the report.

> **A note on the `|| true` after `go test`.** In a real project you would never do this — a failing test must turn CI red, full stop. I use it here only because the suite deliberately ships RED tests documenting unfixed upstream bugs (I'm not allowed to fix the app), and without it CI would be permanently red on findings I *want* to keep visible. The `|| true` keeps the GitHub Actions UI green so the Allure report always publishes; the red tests live *in the report*, not in the build status. The honest version of this — gate on "did the count of passing tests regress" instead of swallowing all failures — is the first item under *With more time*.

## Bonus — testing a non-deterministic GenAI feature

Say Conduit added an LLM that auto-summarises articles or suggests tags. The way I think about LLM testing: you **can't test `equals`** — you test that the output *looks similar and adequate*. So I'd layer techniques, strict-first, and the combination is what makes up the **eval**:

- **What *can* be asserted strictly, assert strictly.** Is the response valid JSON? Right shape — 1–5 tags, summary non-empty and under N chars, same language as the source, tags drawn from the article's vocabulary? These are cheap, deterministic, and catch the dumb failures first.
- **Grounding, to catch hallucination.** The summary's claims and entities should appear in the source — assert the output is *entailed by* the input rather than string-matching a golden answer.
- **The fuzzy part — model-graded evals.** For quality that no rule captures (is the summary actually *good*?), use an LLM judge scoring relevance/faithfulness on a rubric over a fixed dataset. The judge is itself noisy, so I'd track the *aggregate* pass rate against a threshold, not per-run exactness.
- **Regression hygiene.** Pin temperature low and seed where the API allows, keep a small golden set for human eyeballs, and use metamorphic checks as cheap signal (paraphrasing an article shouldn't wildly flip the suggested tags).

Put together, that's the eval: the mindset shifts from *"equals expected"* to *"is this output acceptable and grounded,"* measured statistically rather than asserted exactly.

## With more time

In rough priority order:

1. **Contract testing against the RealWorld OpenAPI spec.** The `c.Params`-vs-`c.Query` defect (bug #1) is a *class* of bug; a schema/contract check over every endpoint would catch the whole family systematically instead of one feature at a time.
2. **A proper negative-auth matrix.** Expired, tampered, and wrong-algorithm JWTs against every protected route — I have the seam (the `utils/jwt` unit test) but not the end-to-end coverage.
3. **Boundary & concurrency cases:** double-favorite, follow-self, unfollow-when-not-following, comment on a deleted article, pagination past the end.
4. **A real CI quality gate:** fail the build if the count of *passing* tests regresses, so a newly-broken feature can't sneak through behind `|| true`.
5. **Dev ergonomics:** a Taskfile wrapping lint + coverage, and helper sugar to unify the test style.

─────────────────────────────────────────────────────────────────────────

# ![RealWorld Example App](logo.png)

> ### [Golang/Fiber](https://gofiber.io) codebase containing real world examples (CRUD, auth, advanced patterns, etc) that adheres to the [RealWorld](https://github.com/gothinkster/realworld) spec and API. Project based on [RealWorld example](https://github.com/xesina/golang-echo-realworld-example-app/) for [Golang/Echo](https://echo.labstack.com/)


### [Demo](https://demo.realworld.io/)&nbsp;&nbsp;&nbsp;&nbsp;[RealWorld](https://github.com/gothinkster/realworld)


This codebase was created to demonstrate a fully fledged fullstack application built with Golang/Fiber including CRUD operations, authentication, routing, pagination, and more.

We've gone to great lengths to adhere to the [Golang/Fiber](https://gofiber.io) community styleguides & best practices.

For more information on how to this works with other frontends/backends, head over to the [RealWorld](https://github.com/gothinkster/realworld) repo.


## Quick start

Before quick start you must install [docker](https://www.docker.com), [docker-compose](https://docs.docker.com/compose/)  and [Git](https://git-scm.com/).

**Starts ready docker container**

```bash
mkdir database && chmod o+w ./database && docker run -d -p 8585:8585 -v $(pwd)/database:/myapp/database alpody/golang-fiber-real-world 
```

**Builds and tests**

```bash
git clone https://github.com/alpody/golang-fiber-realworld-example-app.git
cd golang-fiber-realworld-example-app 
chmod a+x start.sh
./start.sh
```
Press <code>Ctrl + c</code> for stop application.

See asciinema this process:

[![asciicast](https://asciinema.org/a/eyZ5upSyv9IJyE36g4sj3ZBBw.svg)](https://asciinema.org/a/eyZ5upSyv9IJyE36g4sj3ZBBw)

## Getting started

### Install Golang (go1.11+)

Please check the official golang installation guide before you start. [Official Documentation](https://golang.org/doc/install)
Also make sure you have installed a go1.11+ version.

### Environment Config

make sure your ~/.*shrc have those variable:

```bash
➜  echo $GOPATH
/Users/xesina/go
➜  echo $GOROOT
/usr/local/go/
➜  echo $PATH
...:/usr/local/go/bin:/Users/alpody/test/bin:/usr/local/go/bin
```

For more info and detailed instructions please check this guide: [Setting GOPATH](https://github.com/golang/go/wiki/SettingGOPATH)

### Clone the repository

Clone this repository:

```bash
➜ git clone https://github.com/alpody/golang-fiber-realworld-example-app.git
```

Or simply use the following command which will handle cloning the repo:

```bash
➜ go get -u -v github.com/alpody/golang-fiber-realworld-example-app
```

Switch to the repo folder

```bash
➜ cd $GOPATH/src/github.com/alpody/golang-fiber-realworld-example-app
```

### Working with makefile

If you had installed make utility, you can simply run and select command. 

```bash
make help
```

### Install dependencies

```bash
➜ go mod download
```

### Run

```bash
➜ go run main.go
```

### Build

```bash
➜ go build
```

### Tests

```bash
➜ go test ./...
```
### Swagger UI

Open url http://localhost:8585/swagger/index.html in browser.

![2021-10-07_17-01-27](https://user-images.githubusercontent.com/13846803/136400503-fedd869c-4508-4699-a79b-66e0bbd765e2.png)


