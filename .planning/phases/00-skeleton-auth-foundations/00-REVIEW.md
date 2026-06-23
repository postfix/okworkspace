---
phase: 00-skeleton-auth-foundations
reviewed: 2026-06-18T00:00:00Z
depth: standard
files_reviewed: 39
files_reviewed_list:
  - cmd/okf-workspace/admin.go
  - cmd/okf-workspace/main.go
  - deploy/Dockerfile
  - deploy/okf-workspace.service
  - internal/audit/audit.go
  - internal/auth/auth.go
  - internal/auth/password.go
  - internal/auth/rbac.go
  - internal/auth/session.go
  - internal/auth/sqlitestore.go
  - internal/config/config.go
  - internal/gitstore/commit.go
  - internal/gitstore/git.go
  - internal/gitstore/health.go
  - internal/jobs/queue.go
  - internal/jobs/worker.go
  - internal/repo/files.go
  - internal/repo/path.go
  - internal/server/handlers_auth.go
  - internal/server/handlers_health.go
  - internal/server/handlers_profile.go
  - internal/server/handlers_users.go
  - internal/server/middleware.go
  - internal/server/router.go
  - internal/store/db.go
  - internal/store/migrations.go
  - internal/store/migrations/0001_init.sql
  - internal/store/migrations/0002_jobs.sql
  - internal/store/migrations/0003_audit.sql
  - internal/users/bootstrap.go
  - internal/users/manage.go
  - internal/users/seed.go
  - internal/users/users.go
  - internal/web/embed.go
  - web/src/api/client.ts
  - web/src/App.tsx
  - web/src/routes/Admin.tsx
  - web/src/routes/AppShell.tsx
  - web/src/routes/Login.tsx
  - web/src/routes/Profile.tsx
  - web/src/routes/ForcePasswordChange.tsx
  - web/vite.config.ts
findings:
  critical: 3
  warning: 7
  info: 4
  total: 14
status: resolved
---

# Phase 0: Code Review Report

**Reviewed:** 2026-06-18T00:00:00Z
**Depth:** standard
**Files Reviewed:** 39
**Status:** issues_found

## Summary

This is the security-critical auth/storage foundation. The path-safety resolver (SEC-01), Argon2id hashing, parameterized SQL, gitstore argument-slice shell-out, and audit secret-redaction are all implemented correctly and defensively — those areas are genuinely strong, with good fuzz/symlink test coverage.

However, three correctness/authorization defects are serious enough to block:

1. The `must_change_password` gate is enforced **only in the React client** — the backend authorizes every endpoint (including all admin user-management routes) for a user holding a temporary password. (BLOCKER)
2. The one-time-password generator uses `byte % len(alphabet)`, introducing **modulo bias** that skews and reduces the effective entropy of every bootstrap/created/reset credential. (BLOCKER)
3. There is **no last-admin / self-lockout protection**: an admin can demote or deactivate the last (or only) admin — including themselves — permanently locking the entire instance out of all admin functions. (BLOCKER)

Warnings cover a missing username validation surface feeding the Git author identity, a deactivated-but-already-logged-in user retaining their session, audit failure semantics, and several robustness gaps. Performance items are out of scope per the brief.

## Critical Issues

### CR-01: `must_change_password` is enforced client-side only — server authorizes temp-password users for all actions

**File:** `internal/server/handlers_users.go:56-71` (`loadCurrentUser`), `internal/server/router.go:73-91`, `web/src/App.tsx:36-39`
**Issue:** The forced-password-change gate (D-02, T-00.03-04) is implemented exclusively in the SPA (`RequireAuth`/`RequireAdmin` render `<ForcePasswordChange/>` when `must_change_password` is true). The backend `loadCurrentUser` middleware resolves the session user and checks only `u.Active` — it never inspects `MustChangePassword`. A user who signs in with a one-time/temporary password obtains a fully valid session and can call **any** authenticated endpoint directly (e.g. `PUT /api/v1/profile`, and if their role is admin, every `/api/v1/admin/users/*` route) by skipping the SPA and issuing the HTTP request themselves. The code comment in `App.tsx` even claims "the gate cannot be skipped by navigating directly … enforced from the server" — which is false; nothing server-side enforces it. This is a privilege/authorization gap: the temporary-credential state is not a real security boundary.
**Fix:** Enforce the flag server-side. In `loadCurrentUser`, after loading the user, if `u.MustChangePassword` is true, reject every request except the self-service password change:
```go
ctx := auth.WithCurrentUser(r.Context(), sessionUser{id: u.ID, role: u.Role})
if u.MustChangePassword && r.URL.Path != "/api/v1/profile/password" {
    writeError(w, http.StatusForbidden, "Set a new password to continue.")
    return
}
next.ServeHTTP(w, r.WithContext(ctx))
```
(Adjust the exemption to match the change-password route exactly, and keep `GET /auth/me` reachable so the SPA can still read the flag.)

### CR-02: One-time password generator has modulo bias, reducing credential entropy

**File:** `internal/users/bootstrap.go:67-77`
**Issue:** `generatePassword` does `out[i] = passwordAlphabet[int(b)%len(passwordAlphabet)]`. The alphabet length is 56; 256 is not a multiple of 56, so byte values 0–255 map non-uniformly onto the alphabet (the first `256 mod 56 = 32` characters are ~1.27× more likely than the rest). This biases every bootstrap-admin password, every created-user OTP, and every reset OTP (all routed through this function), lowering effective entropy and creating a predictable distribution. For credentials that are the sole gate to an admin account, biased generation is a real cryptographic weakness (and contradicts the "strong random" claims in the doc comments).
**Fix:** Use rejection sampling or `crypto/rand` over the alphabet without modulo. Simplest correct form:
```go
import "crypto/rand"

func generatePassword(length int) (string, error) {
    out := make([]byte, length)
    max := byte(len(passwordAlphabet))
    threshold := byte(256 - (256 % len(passwordAlphabet))) // largest unbiased ceiling
    buf := make([]byte, 1)
    for i := 0; i < length; {
        if _, err := rand.Read(buf); err != nil {
            return "", err
        }
        if buf[0] >= threshold {
            continue // reject to remove modulo bias
        }
        out[i] = passwordAlphabet[buf[0]%max]
        i++
    }
    return string(out), nil
}
```
(Or use `crypto/rand.Int` with `big.Int` bounds.)

### CR-03: No last-admin / self-lockout protection on role change and deactivate

**File:** `internal/users/manage.go:108-145` (`SetRole`, `Deactivate`), `internal/server/handlers_users.go:159-247` (`handleSetRole`, `handleDeactivate`)
**Issue:** Neither `SetRole` nor `Deactivate` guards against removing the last administrator. An admin can:
- demote the only admin to `reader`/`editor` (`SetRole(self, "reader")`), or
- deactivate the only admin account (including their own — `handleDeactivate` derives the target id from the URL with no "not self" / "not last admin" check).

Either action leaves the instance with zero active admins. Because admin bootstrap (`BootstrapAdmin`) is a strict no-op once any user exists, there is no in-app recovery — the only escape is the shell-access `admin reset-password` CLI, and even that cannot re-grant the admin role. This is a data/availability-loss class defect: a single misclick permanently disables all user management. (The admin UI does not even hide the Deactivate button for the current user — `Admin.tsx:170-178` shows it for every active user.)
**Fix:** Enforce an invariant in the management layer (server-side, not just UI): before demoting or deactivating an admin, count remaining *active admins* and reject if this action would drop it to zero. Example for `Deactivate`:
```go
func Deactivate(ctx context.Context, repo *Repository, id int64) error {
    u, err := repo.GetByID(ctx, id)
    if err != nil { return err }
    if u.Role == RoleAdmin {
        n, err := repo.CountActiveAdmins(ctx)
        if err != nil { return err }
        if n <= 1 { return ErrLastAdmin }
    }
    return repo.SetActive(ctx, id, false)
}
```
Apply the equivalent guard in `SetRole` when demoting an admin, map `ErrLastAdmin` to a 400/409 in the handlers, and hide/disable the self-deactivate control in `Admin.tsx`.

## Warnings

### WR-01: Username is never validated; flows unsanitized into the Git author identity and `-c user.name=`

**File:** `internal/users/manage.go:64-100` (`Create`), `internal/gitstore/commit.go:49-61`
**Issue:** `Create` trims the username and checks non-empty, but applies no character/format/length validation. The username later becomes `spec.User`, which `Commit` interpolates into `-c user.name=<username>` and `--author "<username> <email>"`. Because args are passed as a slice (good — no shell injection), an attacker cannot inject new git arguments. But a username containing newlines, control characters, or `=`/`<`/`>` can corrupt the git config token / author header for that commit and pollute the git-history audit trail (`buildMessage` also embeds `User:` verbatim into the commit body). It also allows confusing/duplicate-looking usernames (leading/trailing Unicode spaces, etc.). Username is security-relevant identity data and should be constrained.
**Fix:** Validate username in `Create` (and reject on update paths): enforce a charset/length, e.g. `^[A-Za-z0-9._-]{1,64}$`, and reject control characters and whitespace. Return a typed error mapped to a 400.

### WR-02: Deactivating or role-changing a user does not invalidate existing sessions

**File:** `internal/server/handlers_users.go:56-71` (`loadCurrentUser`), `internal/users/manage.go:140-145` (`Deactivate`)
**Issue:** `loadCurrentUser` re-loads the user every request and rejects `!u.Active`, so a deactivated user's *next* request is blocked — good for deactivation. However, a **role change** is also re-read each request (also fine), but neither operation revokes the SCS session token. More importantly, there is no mechanism to forcibly terminate a compromised/abused active session — the only protection for deactivation is the per-request `Active` check, and for a password reset (`ResetPassword`/admin) the victim's existing logged-in session remains fully valid (reset sets `must_change_password` but, per CR-01, that is not enforced server-side, so the old session keeps working with full privileges). An admin resetting a compromised account's password does not actually kick the attacker out.
**Fix:** On `Deactivate` and `ResetPassword`, delete the target user's session rows (the `sessions` table is in the same DB; SCS exposes `Iterate`/store deletion, or add a `DELETE FROM sessions` keyed on the serialized user id). At minimum, fixing CR-01 makes a password reset force a re-auth via the temp-password gate.

### WR-03: `markDone` increments `attempts`, distorting retry accounting and metrics

**File:** `internal/jobs/queue.go:108-114`, `internal/jobs/worker.go:128-139`
**Issue:** `markDone` runs `attempts=attempts+1` on success. `drainOne` computes `nextAttempt := jr.attempts + 1` and compares to `MaxAttempts` for the retry/fail decision, so the persisted `attempts` value after a success no longer reflects the number of *failed* tries (a job that succeeded on the first try shows `attempts=1`). This is harmless to control flow today but corrupts the audit/diagnostic meaning of the column and will mislead any future "how many retries did this job take" logic. Inconsistent with `markRetry`/`markFailed` which legitimately count attempts.
**Fix:** Do not increment `attempts` in `markDone` (drop `attempts=attempts+1`); leave the count as the number of attempts actually consumed before success, or define the semantics explicitly and apply consistently.

### WR-04: `claimNextDue` returns `(jobRow{}, sql.ErrNoRows)` indistinguishably from real DB errors via empty-on-error

**File:** `internal/jobs/worker.go:112-119`
**Issue:** `drainOne` treats `sql.ErrNoRows` as "queue empty" (returns false) and **all other errors** as "transient, retry next tick" (also returns false, silently). A persistent error (e.g. schema mismatch, corrupted row, context cancellation mid-claim) is swallowed with no log, so the worker spins every `PollInterval` forever without surfacing the failure. There is no observability into a wedged worker.
**Fix:** Log non-`ErrNoRows` errors from `claimNextDue` (and `markDone`/`markRetry`/`markFailed`, whose errors are also currently discarded with `_ =`) at warn level so a persistently failing queue is visible.

### WR-05: `handleHealth` masks real health-check errors as a generic healthy-ish 200

**File:** `internal/server/handlers_health.go:31-36`
**Issue:** When `RepoHealth` returns an error, the handler responds `200 OK` with `RepoHealth{OK:false, Detail:"Storage health check failed"}` and discards the underlying error entirely (no log). A failing `git status` / unreachable repo is reported to the client as a soft "not ok" with no server-side trace, making operational diagnosis hard. Also, `gitstore.Health` returns `OK:false` but the SPA (`AppShell.tsx:50`) only renders a banner for `diverged`/`self_healed`; a plain `ok:false` produces no user-visible signal at all.
**Fix:** Log the error server-side (`slog.Error`) before responding, and ensure the SPA surfaces a generic `!ok` health state (not only the diverged/self_healed sub-cases).

### WR-06: Login does not block on `must_change_password` and SPA passes the temp password through component state

**File:** `internal/server/handlers_auth.go:74-117`, `web/src/App.tsx:36-38`, `web/src/routes/ForcePasswordChange.tsx:13-22`
**Issue:** `handleLogin` succeeds and establishes a full session regardless of `must_change_password`, returning the flag in the body. The SPA then renders `<ForcePasswordChange/>` *without* passing the just-used password (`App.tsx` renders `<ForcePasswordChange />` with no `currentPassword` prop), so the user must re-type the temporary password — but the deeper issue is that the session is already privileged (see CR-01). Independently, the `ForcePasswordChange` component is designed to accept the plaintext temp password as a prop and seed it into React state; threading plaintext credentials through component props/state is a fragile pattern that risks leaking into React devtools / error boundaries / logging.
**Fix:** Primarily fix CR-01 so the session is gated server-side. Keep `ForcePasswordChange` requiring the user to re-enter the temporary password (the path already used in `App.tsx`) and avoid passing plaintext credentials through props/state.

### WR-07: `withDefaults`/config and `secure` HTTPS detection logic duplicated and brittle

**File:** `internal/server/router.go:105-106`, `internal/auth/session.go:32`
**Issue:** Two different code paths decide cookie `Secure`: `auth.NewSessionManager` uses `strings.HasPrefix(strings.ToLower(publicURL), "https://")`, while `server.New` computes `secure` with a hand-rolled `len(...) >= 8 && publicURL[:8] == "https://"` (case-sensitive, no lowercasing). For a config like `HTTPS://host`, the session cookie would be marked Secure but the nosurf CSRF cookie would **not** (case-sensitive check fails), producing inconsistent cookie security flags. Divergent duplicated logic for a security-relevant flag is a defect waiting to bite.
**Fix:** Compute `secure` once (case-insensitive `HasPrefix`) in a single helper and pass it to both the session manager and `csrfProtect`.

## Info

### IN-01: Empty `tags: []` frontmatter emits invalid YAML indentation

**File:** `internal/users/seed.go:109-126`
**Issue:** In `page`, when `tags` is empty, `tagLines` becomes `"  []\n"`, which is spliced after the `tags:\n` line, producing:
```
tags:
  []
```
That is `tags: null` with a stray `[]` child mapping rather than `tags: []`. All current seed pages pass non-empty tags so this branch is never hit, but it is latent-broken frontmatter generation.
**Fix:** Emit `tags: []` on the same line as the key when empty, or build the YAML with `yaml.Marshal` rather than `fmt.Sprintf` string templating.

### IN-02: `users.SetActive` can reactivate but no API exposes it; dead capability

**File:** `internal/users/users.go:155-158`
**Issue:** `SetActive(..., true)` (reactivate) exists at the repo layer but no management function or handler ever calls it with `true` — the admin UI/copy references reactivation ("until reactivated") but there is no reactivate path. Either dead code or a missing feature.
**Fix:** Either add a reactivate management function + handler, or remove the reactivate affordance from the UI copy until it exists.

### IN-03: `dummyHash` can be empty if init-time hashing fails, weakening timing equalization

**File:** `internal/auth/auth.go:35-43,52-55`
**Issue:** If `HashPassword` fails at init, `dummyHash` stays `""`. `VerifyPassword("", plain)` on the unknown-user path then returns near-instantly (no Argon2 work), defeating the constant-time/no-enumeration goal for that edge. Unlikely, but the fallback comment ("Verify still runs in constant-ish time") is inaccurate — an empty hash does not run the KDF.
**Fix:** Treat an init hashing failure as fatal (panic at startup) since a working hasher is a hard requirement, or precompute a hardcoded valid Argon2id PHC string as the fallback.

### IN-04: `repo.Tree` walks the real filesystem and can follow symlinks out of the tree in listings

**File:** `internal/repo/files.go:81-110`
**Issue:** `Tree` uses `filepath.WalkDir` directly on `r.root` (not through `Resolve`/`os.Root`). `WalkDir` does not follow symlinked directories for recursion, so escape is limited, but it will still *list* symlink entries that point outside the root, and the listing bypasses the SEC-01 chokepoint the package docstring claims is universal ("no other package constructs absolute repo paths"). Currently only used for seed verification, low impact this phase.
**Fix:** When the tree API is promoted beyond seed verification (Phase 1), skip symlinks during the walk or validate each entry through the resolver, and note the current limitation.

---

## Resolution

Resolved 2026-06-18. All 3 Critical/Blocker findings and all 7 Warnings are
fixed; the 4 Info findings (IN-01..IN-04) are deferred to a later phase.

Gate after fixes (all green):
- `CGO_ENABLED=0 go build ./...` — pass
- `CGO_ENABLED=0 go test ./internal/... ./cmd/... -count=1` — pass (all packages ok)
- `go vet ./internal/... ./cmd/...` — pass
- `cd web && npm run build && npm run lint` — pass

| Finding | Status | Commit | Notes |
|---------|--------|--------|-------|
| CR-01 | Fixed | c822055 | `loadCurrentUser` rejects all authenticated routes (403) except `PUT /api/v1/profile/password` while `must_change_password` is set; `/auth/me` stays reachable. Regression test `TestMustChangePasswordGate`. |
| CR-02 | Fixed | 1c1701a | `generatePassword` uses rejection sampling over `crypto/rand` (no modulo bias); same alphabet/length/error path. |
| CR-03 | Fixed | 7bd9d2d, c822055, ec44e4f | Added `CountActiveAdmins` + `ErrLastAdmin`; `SetRole`/`Deactivate` reject dropping active admins to zero (→409 in handlers); Admin UI hides Deactivate for self and last admin. Regression test `TestLastAdminInvariant`. |
| WR-01 | Fixed | 7bd9d2d, c822055 | Username validated against `^[A-Za-z0-9._-]{1,64}$` in `Create` (`ErrInvalidUsername` → 400). Test `TestCreateRejectsInvalidUsername`. |
| WR-02 | Fixed | 7bd9d2d | `DeleteSessionsForUser` decodes SCS gob session blobs and revokes the target's sessions on `Deactivate`/`ResetPassword` (best-effort; CR-01 already forces re-auth on reset). |
| WR-03 | Fixed | ec44e4f | `markDone` no longer increments `attempts`. |
| WR-04 | Fixed | ec44e4f | `Worker` gains a slog logger; non-`ErrNoRows` claim errors and previously-discarded mark* errors are logged at warn; wired in `main`. |
| WR-05 | Fixed | ec44e4f | `handleHealth` `slog.Error`s the underlying error; `AppShell` surfaces a generic `!ok` storage banner. |
| WR-06 | Fixed | ec44e4f | Server gate (CR-01) is authoritative; `ForcePasswordChange` no longer accepts a plaintext-password prop — the user re-enters the temp password. |
| WR-07 | Fixed | ec44e4f | Single `auth.SecureCookies` (case-insensitive `https://`) drives both the session and CSRF cookie `Secure` flags. |
| IN-01 | Deferred | — | Empty `tags: []` YAML indentation; latent path, deferred. |
| IN-02 | Deferred | — | Unexposed `SetActive(true)` reactivate; deferred. |
| IN-03 | Deferred | — | Empty `dummyHash` timing edge; deferred. |
| IN-04 | Deferred | — | `repo.Tree` symlink listing; deferred to Phase 1 per finding. |

---

_Reviewed: 2026-06-18T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
