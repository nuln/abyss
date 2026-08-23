# Authentication & Permissions

## Description
Context on the JWT-based authentication and the permission model used in the Abyss backend.

## Context
- `identity.go` (was auth.go): Core authentication logic and middleware.
- `identity.go` (was user.go): User roles and definitions.
- `app.go`: Global auth service initialization.

## Instructions
1. **JWT Authentication**:
   - Abyss uses HS256 JWTs for authentication.
   - The token should be passed in the `X-Auth` header or `Authorization: Bearer <token>`.
   - Claims include `uid` (UserID), `role` (UserRole), and `admin` (Boolean).
2. **Middleware**:
   - `authMiddleware` validates tokens and injects `UserID` and `IsAdmin` into the request context.
   - Use `AuthUserID(ctx)` and `AuthIsAdmin(ctx)` to retrieve these values in handlers.
3. **Roles**:
   - `RoleAdmin`: Full access to the system.
   - `RoleUser`: Standard user permissions.
   - `RoleGuest`: Restricted access.
4. **Audit & Debugging**:
   - If a request returns 403 Forbidden, check if the `authMiddleware` is applied and if the user's role satisfies the handler's requirements.
   - Logs for auth failures are usually recorded via the `slog` logger with appropriate context.
5. **MFA Flow**:
   - Plugins can hook into `OnLoginSuccess` to trigger MFA (e.g., TOTP).
   - The frontend must handle the `AuthResult` and redirect to the MFA verification page if needed.
