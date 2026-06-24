# ADR-0002: token-strategy

**Status**: Proposed
**Version**: 1.0
**Date**: 2026-06-24
**Author**: Ramdan Agus Saputra

## Context

Selaras needs production-credible authentication that a mid-level interviewer will probe in depth (see [PRD](../selaras-prd.md) Story 1 / Feature 1 and [spec 02-auth](../specs/02-auth.md)). The forces in tension:

- **Stateless, horizontally-scalable request auth** vs **instant revocation** — a JWT validated by signature alone needs no per-request database read, but cannot be revoked before it expires.
- **Browser threat model** — the app is a React SPA, so any credential reachable by JavaScript is exposed to XSS, and any credential the browser sends automatically is exposed to CSRF.
- **Theft detection without a session server** — refresh tokens are long-lived; a stolen one must be detectable, but the design must also tolerate the benign concurrent-refresh races that a multi-tab SPA naturally produces.
- **Reproducibility and cost** — password hashing parameters must be strong, deterministic across environments, and survive future cost bumps without invalidating stored hashes. There is no Redis or session store (single instance, [ADR-0004 territory]); the only durable store is Postgres.

This ADR records the four coupled decisions made for M2: password hashing, access-token format, refresh-token rotation + reuse detection, and the browser storage / CSRF posture.

## Decision

**We will split credentials by lifetime and store each where it is hardest to steal, hash passwords with argon2id, and detect refresh-token theft with a token-family state machine that tolerates a short grace window.**

1. **Password hashing — argon2id, fixed PHC-encoded parameters.** Passwords are hashed with argon2id at `m = 64 MiB, t = 2, p = 1`, stored as the full self-describing PHC string (`$argon2id$v=19$m=65536,t=2,p=1$<salt>$<key>`). Verification parses the cost parameters back out of the stored hash. Parallelism is pinned to `1` (not host-core-derived) for cross-environment determinism. Login is timing-safe: an unknown email is verified against a fixed dummy hash so response timing cannot enumerate registered accounts.

2. **Access token — HS256 JWT, short-lived, minimal claims, algorithm-pinned.** Access tokens are HS256 JWTs signed with `JWT_SECRET` (≥32 bytes, fail-fast at boot), carrying exactly `sub`, `iat`, `exp` with a 15-minute TTL. Verification pins the accepted algorithm to HS256 (rejecting `none`/RS256 confusion) and is **stateless** — the auth middleware does no database read and injects the user id into the request `context.Context`.

3. **Refresh token — opaque, rotated, family-grouped, hash-stored.** Refresh tokens are 32 cryptographically-random bytes (base64url); only their SHA-256 is stored. Every refresh rotates: the presented token is revoked and a successor is minted in the **same `family_id`**, with a 168-hour TTL. Reuse detection: presenting an already-revoked token revokes the **entire family** (`401 TOKEN_REUSED`, logged at WARN) — *except* a token revoked within a ~10-second grace window and replayed while its family is still otherwise live, which is tolerated **once** as a benign concurrent-refresh race (a fresh pair is issued, the family is not revoked). Expired families are purged opportunistically on refresh.

4. **Browser storage & CSRF posture.** The access token lives **in memory** in the SPA (a module variable, never `localStorage`/`sessionStorage`) and is sent as an `Authorization: Bearer` header. The refresh token is an **HttpOnly; Secure (prod); SameSite=Lax; Path=/api/v1/auth** cookie. On load the app performs one silent refresh to re-establish a session from the cookie.

## Alternatives considered

- **bcrypt for hashing** — mature and fine, but only mildly memory-hard (~4 KiB), so far cheaper to attack on GPUs/ASICs, and it silently truncates passwords past 72 bytes. argon2id (PHC-2015 winner, OWASP first choice for new code) is the stronger default for greenfield.
- **`p` = host core count** — lets a multi-core box hash faster, but makes the security parameter a property of the deployment hardware rather than a deliberate choice, and makes dev/CI/prod produce divergent hashes. Pinning `p=1` keeps hashing reproducible; cost is tuned via `m`/`t`, which we reason about explicitly.
- **Hardcoding hash params in the verifier** — simpler, but a future cost bump would invalidate every stored hash. Encoding params in the PHC string lets old hashes keep verifying and supports transparent re-hash on next login.
- **Stateful/opaque access tokens (DB or Redis lookup per request)** — gives instant revocation, but adds a hot-path read (and a Redis dependency we don't have at this scale). Rejected in favor of short-lived stateless JWTs; revocation power is concentrated in the stateful refresh layer instead.
- **Long-lived access token, no refresh** — simplest, but a leaked token is valid for its whole life with no rotation or theft signal. Rejected.
- **No reuse detection (plain rotation)** — rotation alone doesn't notice a stolen-then-replayed token. The family state machine adds theft detection. A *strict* version (any revoked-token replay nukes the family, no grace) was rejected because a multi-tab SPA legitimately replays a just-rotated token during concurrent refreshes, which would log honest users out constantly — hence the tolerate-once grace window.
- **Access token in `localStorage`** — survives reloads without a refresh round-trip, but is readable by any XSS. Rejected; in-memory + silent-refresh trades one startup refresh for a materially smaller XSS blast radius.
- **Refresh token as a non-cookie value handled by JS** — would make the API CSRF-immune end-to-end, but exposes the long-lived secret to XSS and requires the SPA to persist it somewhere. Rejected; HttpOnly cookie keeps it out of JS reach. `SameSite=Lax` (not `Strict`) is chosen so following a link into the app still works while cross-site sub-requests carry no cookie; `Path` scoping keeps the cookie off every non-auth request.

## Consequences

**Easier:**
- Request authentication scales horizontally with zero shared session state on the hot path.
- Token theft is detectable and self-healing (family revocation) without a session server.
- The most sensitive credential (refresh) is unreadable by JS; the API surface (Bearer header) is structurally CSRF-immune, leaving only `/auth/refresh` relying on the cookie, which `SameSite=Lax` covers.
- Password hashes are portable across environments and survive cost increases.

**Harder / accepted trade-offs:**
- **Access tokens cannot be revoked before expiry** — accepted because the TTL is 15 minutes and all durable power lives in the revocable refresh layer. "Log out everywhere now" is enforced by revoking refresh families, not by killing access tokens.
- **A full page reload costs one silent refresh round-trip** (the in-memory token is gone) — accepted as the price of keeping the token out of web storage.
- **The grace window is a deliberate, bounded loosening of theft detection** — within ~10 seconds a replayed just-revoked token is tolerated once. Accepted because the alternative (strict) breaks legitimate multi-tab use; the window is short and still requires an otherwise-intact family.
- **`JWT_SECRET` is symmetric** — anyone with the secret can mint tokens, so it must be a protected env var (it is, fail-fast at boot). Asymmetric signing (RS256/EdDSA) is unnecessary for a single service that both issues and verifies.

**Revisit triggers:**
- Move to **multiple API instances** sharing WebSocket fan-out or needing cross-instance token revocation → reconsider a shared store (Redis) and possibly stateful access tokens (relates to the Phase 2 Redis item / ADR-0004).
- Add **third-party / OAuth login** (Phase 2) → the access/refresh model extends, but issuance and account-linking need their own decision.
- Evidence of **argon2id cost being too low/high** for the deployment (login latency or cracking economics) → bump `m`/`t`; the PHC-encoded params make this a transparent migration.
- Need for **immediate access-token revocation** (e.g. compliance) → introduce a short deny-list or shorten the TTL further.
