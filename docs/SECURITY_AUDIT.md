# Librarr Security Audit

Audit date: 2026-06-03  
Scope: Amateur-God/librarr (`review/security` branch)  
Method: Static code review of HTTP handlers, auth flows, file I/O, and outbound HTTP clients.

Findings are grouped by severity. **Fixed in this branch** indicates a code change was merged on `review/security`.

---

## Critical

### C1: SSRF via direct download URL

| | |
|---|---|
| **Files** | `internal/api/download.go`, `internal/download/direct.go` |
| **Scenario** | Authenticated user POSTs `{"url":"http://169.254.169.254/..."}` to `/api/download`; server fetches internal metadata or other private endpoints. |
| **Fix** | **Fixed in this branch** — `netutil.ValidateOutboundURL` applied before `StartDirectDownload`. |

### C2: Arbitrary filesystem read via manual import

| | |
|---|---|
| **Files** | `internal/api/manualimport.go` |
| **Scenario** | User supplies `/etc` or other paths to `/api/import/scan` or `/api/import/files`. |
| **Fix** | **Fixed in this branch** — paths must resolve under configured library/incoming roots (`validateAllowedPath`). |

---

## High

### H1: SSRF via webhook test/create

| | |
|---|---|
| **Files** | `internal/api/webhooks.go`, `internal/webhook/webhook.go` |
| **Scenario** | Admin triggers outbound POST to internal services via webhook test or malicious webhook URL. |
| **Fix** | **Fixed in this branch** — URL validation on create and test; generic error on test failure. |

### H2: Incomplete SSRF guard on connection tests

| | |
|---|---|
| **Files** | `internal/api/settings.go` |
| **Scenario** | Prowlarr connection test accepted `localhost` / private IPs via prefix-only blocklist. |
| **Fix** | **Fixed in this branch** — shared `netutil.ValidateOutboundURL` with DNS/IP checks; applied to Prowlarr, Audiobookshelf, Kavita tests. |

### H3: OPDS library exposure without auth

| | |
|---|---|
| **Files** | `internal/api/auth.go`, `internal/api/opds.go` |
| **Scenario** | When session auth is enabled, `/opds/books` and `/opds/download/{id}` were reachable without login. |
| **Fix** | **Fixed in this branch** — only `/opds`, `/opds/`, and `/opds/opensearch.xml` remain auth-exempt. Books/search/download require session or API key. |

### H4: Session cookie missing Secure flag

| | |
|---|---|
| **Files** | `internal/api/auth.go`, `internal/api/oidc.go` |
| **Scenario** | Session cookie sent over HTTP on TLS-terminated deployments without `Secure`. |
| **Fix** | **Fixed in this branch** — `sessionCookie()` sets `Secure` when TLS or `X-Forwarded-Proto: https`. |

### H5: OIDC email_verified not enforced

| | |
|---|---|
| **Files** | `internal/api/oidc.go`, `internal/config/config.go` |
| **Scenario** | Unverified email from IdP used for account creation/login. |
| **Fix** | **Fixed in this branch** — reject when email present and `email_verified == false` (configurable via `OIDC_REQUIRE_EMAIL_VERIFIED`, default `true`). |

### H6: Error messages leak upstream details

| | |
|---|---|
| **Files** | `internal/api/download.go`, `internal/api/backup.go` |
| **Scenario** | Raw `err.Error()` returned to client may include filesystem paths or upstream URLs. |
| **Fix** | **Fixed in this branch** — generic client messages; details logged server-side. |

### H7: Backup API returns internal filesystem path

| | |
|---|---|
| **Files** | `internal/api/backup.go` |
| **Scenario** | `POST /api/backup/create` JSON included absolute `path` to zip on disk. |
| **Fix** | **Fixed in this branch** — path removed from response. |

---

## Medium

### M1: API key via query parameter

| | |
|---|---|
| **Files** | `internal/api/auth.go` |
| **Scenario** | `?apikey=` may appear in access logs, Referer, browser history. |
| **Fix** | **Partial** — audit documents risk; warning logged when query param used. Prefer `X-Api-Key` header. |

### M2: Login response includes token in JSON body

| | |
|---|---|
| **Files** | `internal/api/auth.go` |
| **Scenario** | Token in body increases XSS exfil surface if UI ever renders untrusted content. |
| **Fix** | **Documented** — cookie is primary mechanism; body token retained for API clients. |

### M3: Torznab API key optional

| | |
|---|---|
| **Files** | `internal/torznab/handler.go` |
| **Scenario** | When `TORZNAB_API_KEY` unset, Torznab endpoint is unauthenticated. |
| **Fix** | **Documented** — set `TORZNAB_API_KEY` in production. |

### M4: File upload extension-only validation

| | |
|---|---|
| **Files** | `internal/api/upload.go` |
| **Scenario** | Malicious file with allowed extension but wrong content. |
| **Fix** | **Deferred** — planned for `review/code-quality` (magic-byte check). |

### M5: TOTP brute force

| | |
|---|---|
| **Files** | `internal/api/totp.go`, `internal/api/auth.go` |
| **Scenario** | No dedicated rate limit on `/api/login/totp`. |
| **Fix** | **Deferred** — general login rate limit exists; dedicated TOTP limit in code-quality branch. |

### M6: Settings credentials at rest

| | |
|---|---|
| **Files** | `settings.json` |
| **Scenario** | API keys/passwords stored plaintext (file mode `0600`). |
| **Fix** | **Documented** — acceptable for single-user homelab; encryption at rest out of scope. |

### M7: LIBRARR_SOURCES_URL SSRF if env compromised

| | |
|---|---|
| **Files** | `internal/sources/load.go` |
| **Scenario** | Attacker with env access points registry fetch at internal URL. |
| **Fix** | **Documented** — admin-controlled env; validate if URL becomes user-settable. |

---

## Low / Informational

### L1: OPDS root catalog remains public

Root `/opds` catalog is still auth-exempt for e-reader discovery. Full OPDS Basic Auth support is a follow-up.

### L2: OIDC sub not stored for account linking

No DB column for OIDC `sub` yet; username collision possible across IdP changes. Document for future migration.

### L3: DNS rebinding

`ValidateOutboundURL` resolves at validation time; time-of-check/time-of-use gap remains. Mitigation: custom `http.Transport` with dial-time IP check (future hardening).

### L4: Webhook delivery goroutines unbounded

`webhook.Sender.Send` spawns a goroutine per delivery. Worker pool planned for code-quality branch.

### L5: Logging

No passwords/API keys found in slog calls. OIDC errors log generic messages.

---

## Positive controls observed

- bcrypt password hashing with reasonable length limits
- Constant-time API key comparison (`subtle.ConstantTimeCompare`)
- Sensitive settings masked on GET (`settings.go`)
- Request body size limits (1MB JSON, 500MB multipart upload)
- OPDS download path confinement with symlink resolution
- SQLite WAL mode + busy timeout
- CI: `go test -race`, govulncheck, staticcheck, CodeQL workflow

---

## Branch follow-up

| Branch | Planned work |
|--------|----------------|
| `review/code-quality` | `writeError` helper, input validation, race fixes, upload magic bytes |
| `review/testing-gaps` | Pipeline integration, fuzz tests, concurrent search tests |
