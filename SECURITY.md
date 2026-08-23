# Security Policy

## Reporting a Vulnerability

Please open a [GitHub Security Advisory](https://github.com/nuln/abyss/security/advisories/new)
for anything that might expose user data or allow authentication bypass.
Do not open a public issue for exploitable problems.

You can expect an initial response within 7 days.

## Security Model

### Authentication

- Dual-token scheme: short-lived **access token** (JWT, HS256) + long-lived
  **refresh token** (opaque random string, stored as SHA-256 hash).
- Access and MFA intermediate tokens are isolated by a `typ` claim; one can
  never be replayed as the other.
- Tokens are accepted from the `X-Auth` header. The `?token=` query parameter
  is **disabled by default** (`allowQueryToken = false`) because URLs leak
  into proxy/access logs.
- Login responses are timing-hardened against user enumeration.

### Plugin crypto API (Users.Encrypt / Users.Decrypt)

- AES-256-GCM with keys derived via HKDF-SHA256.
- A dedicated key can be configured with `auth.encryptionSecret`; when
  absent it falls back to `jwtSecret`. Decryption transparently tries both,
  so rotating to a dedicated key never breaks previously encrypted data.

### File storage

- Every path entering the storage layer is normalised (`path.Clean("/"+p)`)
  and re-rooted by the engine, providing two independent layers of defense
  against path traversal.
- Uploads are capped at 1 GiB per request; JSON bodies at 1 MiB.

## Known Trade-offs

| Item | Status |
|---|---|
| Token storage in `localStorage` | Known trade-off. HttpOnly-cookie migration is planned but requires reworking the plugin token-issuance contract. Primary XSS vectors have been eliminated (EPUB scripting disabled, Markdown sanitised via DOMPurify). |
| `SameSite=Strict` cookie without CSRF tokens | Strict SameSite blocks cross-site carries for all non-GET requests; no state-changing GET endpoints exist. |
| SSE endpoint allows any origin | Read-only task events; auth cookie is SameSite=Strict so it is never attached cross-site. |

## Hardening Recommendations for Production

- Serve over HTTPS only (cookies gain the `Secure` flag automatically).
- Set a dedicated `auth.encryptionSecret` different from `jwtSecret`.
- Keep `allowQueryToken = false`.
- Run `make scan` (govulncheck) periodically — CI does this daily.
