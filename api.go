package abyss

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/nuln/abyss/www"
)

// ── Context keys ──────────────────────────────────────────────────────────────

type authContextKey string

const (
	authUserIDKey  authContextKey = "auth.userID"
	authIsAdminKey authContextKey = "auth.isAdmin"
)

func withAuthUserID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, authUserIDKey, id)
}

func withAuthIsAdmin(ctx context.Context, admin bool) context.Context {
	return context.WithValue(ctx, authIsAdminKey, admin)
}

// AuthUserIDFromContext extracts authenticated user ID from context.
func AuthUserIDFromContext(ctx context.Context) uint64 {
	id, _ := ctx.Value(authUserIDKey).(uint64)
	return id
}

// AuthIsAdminFromContext extracts admin marker from context.
func AuthIsAdminFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(authIsAdminKey).(bool)
	return v
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

// ErrorResponse is the canonical API error payload.
func ErrorResponse(msg string) map[string]string {
	return map[string]string{"error": msg}
}

// WriteJSON writes a JSON response with unified content type.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]any{"success": status < 400}
	if status < 400 {
		payload["data"] = v
	} else {
		switch e := v.(type) {
		case map[string]string:
			payload["error"] = e["error"]
		case map[string]any:
			if msg, ok := e["error"]; ok {
				payload["error"] = msg
			} else {
				payload["error"] = http.StatusText(status)
			}
		default:
			payload["error"] = fmt.Sprint(v)
		}
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteErr maps an application error to HTTP status and writes JSON payload.
func WriteErr(w http.ResponseWriter, err error) {
	var appErr *Error
	msg := ""
	code := http.StatusInternalServerError

	if err != nil && errors.As(err, &appErr) {
		msg = appErr.Message
		switch appErr.Code {
		case ErrNotFound.Code:
			code = http.StatusNotFound
		case ErrUnauthorized.Code:
			code = http.StatusUnauthorized
		case ErrConflict.Code:
			code = http.StatusConflict
		case ErrForbidden.Code:
			code = http.StatusForbidden
		case ErrInvalidInput.Code:
			code = http.StatusBadRequest
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if msg == "" {
		if appErr != nil {
			msg = appErr.Code
		} else {
			msg = http.StatusText(code)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   msg,
	})
}

const maxJSONBodySize int64 = 1 << 20
const maxUploadBodySize int64 = 1 << 30

// DecodeJSON decodes request JSON payload into out.
func DecodeJSON(r *http.Request, out any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodySize+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxJSONBodySize {
		return fmt.Errorf("request body too large")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return io.EOF
	}
	return json.Unmarshal(data, out)
}

// ── Route registration ────────────────────────────────────────────────────────

func registerAllRoutes(r *mux.Router, app *App) {
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slog.Debug("request", "method", r.Method, "url", r.URL.String())
			next.ServeHTTP(w, r)
		})
	})
	authMW := authMiddleware(app.authSvc)

	// Setup status
	r.HandleFunc("/api/setup/status", app.handleSetupStatus).Methods(http.MethodGet)

	// Identity — public routes
	r.HandleFunc("/api/signup", app.handleRegister).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/login", app.handleLogin).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/mfa", app.handleLoginMFA).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/refresh", app.handleRefresh).Methods(http.MethodPost)

	// Identity — protected routes
	r.Handle("/api/auth/logout", authMW(http.HandlerFunc(app.handleLogout))).Methods(http.MethodPost)
	r.Handle("/api/me", authMW(http.HandlerFunc(app.handleMe))).Methods(http.MethodGet)
	r.Handle("/api/me", authMW(http.HandlerFunc(app.handleUpdateMe))).Methods(http.MethodPut)
	r.Handle("/api/auth/sessions", authMW(http.HandlerFunc(app.handleListSessions))).Methods(http.MethodGet)
	r.Handle("/api/auth/sessions/{id}", authMW(http.HandlerFunc(app.handleRevokeSession))).Methods(http.MethodDelete)
	r.Handle("/api/auth/sessions", authMW(http.HandlerFunc(app.handleRevokeAllSessions))).Methods(http.MethodDelete)

	// Resources
	r.Handle("/api/resources/{path:.*}", authMW(http.HandlerFunc(app.handleFileList))).Methods(http.MethodGet)
	r.Handle("/api/resources/{path:.*}", authMW(http.HandlerFunc(app.handleFileUpload))).Methods(http.MethodPost)
	r.Handle("/api/resources/{path:.*}", authMW(http.HandlerFunc(app.handleFileDelete))).Methods(http.MethodDelete)
	r.Handle("/api/resources/{path:.*}", authMW(http.HandlerFunc(app.handleFileRename))).Methods(http.MethodPut, http.MethodPatch)

	// Admin
	r.Handle("/api/users", authMW(http.HandlerFunc(app.handleListUsers))).Methods(http.MethodGet)
	r.Handle("/api/users/{id}", authMW(http.HandlerFunc(app.handleGetUser))).Methods(http.MethodGet)
	r.Handle("/api/users/{id}", authMW(http.HandlerFunc(app.handleUpdateUser))).Methods(http.MethodPut)
	r.Handle("/api/users/{id}", authMW(http.HandlerFunc(app.handleDeleteUser))).Methods(http.MethodDelete)

	// Raw & Preview
	r.PathPrefix("/api/raw/").Handler(authMW(http.HandlerFunc(app.handleFileDownload)))
	r.PathPrefix("/api/preview/").Handler(authMW(http.HandlerFunc(app.handleFilePreview)))

	// Tasks
	r.Handle("/api/tasks", authMW(http.HandlerFunc(app.handleTaskSubmit))).Methods(http.MethodPost)
	r.Handle("/api/tasks", authMW(http.HandlerFunc(app.handleTaskList))).Methods(http.MethodGet)
	r.Handle("/api/tasks/events", authMW(http.HandlerFunc(app.handleTaskEvents))).Methods(http.MethodGet)
	r.Handle("/api/tasks/{id}/cancel", authMW(http.HandlerFunc(app.handleTaskCancel))).Methods(http.MethodPost)

	// Settings
	r.Handle("/api/settings", authMW(http.HandlerFunc(app.handleSettingsGet))).Methods(http.MethodGet)
	r.Handle("/api/settings", authMW(http.HandlerFunc(app.handleSettingsSave))).Methods(http.MethodPut)
	r.Handle("/api/settings/gc", authMW(http.HandlerFunc(app.handleGCStatus))).Methods(http.MethodGet)
	r.Handle("/api/settings/gc", authMW(http.HandlerFunc(app.handleGCRun))).Methods(http.MethodPost)
	r.Handle("/api/settings/storage/migrate", authMW(http.HandlerFunc(app.handleGetMigrationStatus))).Methods(http.MethodGet)
	r.Handle("/api/settings/storage/migrate", authMW(http.HandlerFunc(app.handleStartMigration))).Methods(http.MethodPost)
	r.Handle("/api/settings/storage/migrate/preflight", authMW(http.HandlerFunc(app.handlePreflightMigration))).Methods(http.MethodGet)

	// Plugin HTTP routes
	mountPluginHTTP(r, authMW)

	// Plugin protocol routes
	mountPluginProtocols(r, authMW)
}

// ── Identity handlers ─────────────────────────────────────────────────────────

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}
	settings, _ := a.settingsSvc.Get(r.Context())
	if settings != nil {
		if !settings.Signup {
			WriteJSON(w, http.StatusForbidden, ErrorResponse("signup disabled"))
			return
		}
		if len(req.Password) < settings.MinimumPasswordLength {
			WriteJSON(w, http.StatusBadRequest, ErrorResponse("password too short"))
			return
		}
	}

	u, err := a.userSvc.Register(r.Context(), req.Email, req.Username, req.Password, req.DisplayName)
	if err != nil {
		WriteErr(w, err)
		return
	}

	// Apply default settings for new user
	if settings != nil {
		u.Preferences.Locale = settings.Defaults.Locale
		u.Preferences.Theme = settings.Defaults.Theme
		u.Preferences.Scope = settings.Defaults.Scope
		u.Preferences.SingleClick = settings.Defaults.SingleClick
		u.Preferences.Sorting = settings.Defaults.Sorting
		if err := a.userSvc.UpdateUser(r.Context(), u); err != nil {
			slog.Warn("apply default preferences after register", "user", u.ID, "error", err)
		}
	}
	u.Sanitize()
	WriteJSON(w, http.StatusCreated, u)
}

func (a *App) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	users, err := a.userSvc.List(r.Context())
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"initialized": len(users) > 0})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method   string                 `json:"method"`
		Email    string                 `json:"email"`    // For password method
		Password string                 `json:"password"` // For password method
		Data     map[string]interface{} `json:"data"`     // For plugin methods
	}
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}

	var userID uint64
	var err error

	// 1. Primary Authentication
	method := req.Method
	if method == "" || method == "password" {
		slog.Debug("login attempt", "identifier", req.Email, "method", "password")
		u, loginErr := a.userSvc.GetByEmail(r.Context(), req.Email)
		if loginErr != nil {
			if errors.Is(loginErr, ErrNotFound) {
				// Try by username
				u, loginErr = a.userSvc.GetByUsername(r.Context(), req.Email)
			}
			if loginErr != nil {
				slog.Debug("login failed: user not found", "identifier", req.Email, "err", loginErr)
				WriteJSON(w, http.StatusUnauthorized, ErrorResponse("invalid email or password"))
				return
			}
		}
		if !comparePassword(u.PasswordHash, req.Password) {
			slog.Debug("login failed: password mismatch", "email", req.Email)
			WriteJSON(w, http.StatusUnauthorized, ErrorResponse("invalid email or password"))
			return
		}
		userID = u.ID
	} else {
		// Delegate to plugin
		err = CallAuthenticator(func(p Authenticator) error {
			uid, authErr := p.Authenticate(method, req.Data)
			if authErr == nil {
				userID = uid
			}
			return authErr
		})
		if err != nil {
			WriteErr(w, err)
			return
		}
	}

	if userID == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("authentication failed"))
		return
	}

	// 2. Post-Login Hooks (MFA Check)
	var mfaRes *PluginAuthResult
	_ = CallAuthenticator(func(p Authenticator) error {
		res, hookErr := p.OnLoginSuccess(userID, r)
		if hookErr == nil && res != nil && res.NeedMFA {
			mfaRes = res
		}
		return hookErr
	})

	if mfaRes != nil {
		// MFA Required
		WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"needMFA":  true,
			"mfaToken": mfaRes.MFAToken,
			"method":   mfaRes.MFAMethod,
		})
		return
	}

	// 3. Final Token Issuance
	u, err := a.userSvc.GetByID(r.Context(), userID)
	if err != nil {
		WriteErr(w, err)
		return
	}
	a.issueLoginToken(w, r, u)
}

func (a *App) handleLoginMFA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string                 `json:"mfaToken"`
		Method   string                 `json:"method"`
		Data     map[string]interface{} `json:"data"`
	}
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}

	slog.Debug("mfa.request", "method", req.Method, "token_len", len(req.MFAToken))

	// 1. Verify MFA Token and bound method
	userID, boundMethod, err := a.users.VerifyMFAToken(req.MFAToken)
	if err != nil {
		slog.Info("mfa.verify_token.failed", "err", err)
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("invalid mfa token"))
		return
	}

	slog.Info("mfa.token_ok", "userID", userID, "boundMethod", boundMethod)

	// 2. Validate requested method matches bound method
	if req.Method != boundMethod {
		slog.Info("mfa.method_mismatch", "requested", req.Method, "bound", boundMethod)
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("method mismatch for this token"))
		return
	}

	// 2. Delegate MFA Verification to Plugin
	var success bool
	err = CallAuthenticator(func(p Authenticator) error {
		ok, vErr := p.VerifyMFA(userID, req.Method, req.Data)
		slog.Info("mfa.verify_plugin", "plugin", p.Info().SlugName, "method", req.Method, "ok", ok, "err", vErr)
		if vErr == nil && ok {
			success = true
		}
		return vErr
	})

	if err != nil {
		slog.Info("mfa.plugin_error", "err", err)
		WriteErr(w, err)
		return
	}

	if !success {
		slog.Info("mfa.verify.failed", "userID", userID, "method", req.Method)
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("mfa verification failed"))
		return
	}

	// 3. Final Token Issuance
	u, err := a.userSvc.GetByID(r.Context(), userID)
	if err != nil {
		WriteErr(w, err)
		return
	}
	a.issueLoginToken(w, r, u)
}

func (a *App) issueLoginToken(w http.ResponseWriter, r *http.Request, user *User) {
	res, err := a.authSvc.issueAuthResult(r.Context(), user, r.UserAgent())
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, res)
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}
	result, err := a.authSvc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}
	_ = a.authSvc.Logout(r.Context(), req.RefreshToken)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	u, err := a.authSvc.Me(r.Context(), uid)
	if err != nil {
		WriteErr(w, err)
		return
	}
	u.Sanitize()
	WriteJSON(w, http.StatusOK, u.ToFrontend())
}

func (a *App) handleListSessions(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	sessions, err := a.authSvc.ListSessions(r.Context(), uid)
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, sessions)
}

func (a *App) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	id := mux.Vars(r)["id"]
	session, err := a.sessionStore.GetByID(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}
	if session.UserID != uid && !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("cannot revoke another user's session"))
		return
	}
	if err := a.authSvc.RevokeSession(r.Context(), id); err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	if err := a.authSvc.RevokeAllSessions(r.Context(), uid); err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	existing, err := a.authSvc.Me(r.Context(), uid)
	if err != nil {
		WriteErr(w, err)
		return
	}
	var patch struct {
		DisplayName *string      `json:"displayName"`
		Preferences *Preferences `json:"preferences"`
	}
	if decodeErr := DecodeJSON(r, &patch); decodeErr != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}
	if patch.DisplayName != nil {
		existing.DisplayName = *patch.DisplayName
	}
	if patch.Preferences != nil {
		existing.Preferences = *patch.Preferences
	}
	if err := a.userSvc.UpdateUser(r.Context(), existing); err != nil {
		WriteErr(w, err)
		return
	}
	existing.Sanitize()
	WriteJSON(w, http.StatusOK, existing.ToFrontend())
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
		return
	}
	users, err := a.userSvc.List(r.Context())
	if err != nil {
		WriteErr(w, err)
		return
	}
	frontendUsers := make([]map[string]any, 0, len(users))
	for _, u := range users {
		u.Sanitize()
		frontendUsers = append(frontendUsers, u.ToFrontend())
	}
	WriteJSON(w, http.StatusOK, frontendUsers)
}

func (a *App) handleGetUser(w http.ResponseWriter, r *http.Request) {
	if !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
		return
	}
	id, err := parseIDVar(r, "id")
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid id"))
		return
	}
	u, err := a.userSvc.GetByID(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}
	u.Sanitize()
	WriteJSON(w, http.StatusOK, u.ToFrontend())
}

//nolint:gocyclo // This handler intentionally applies role-aware patch logic in one place.
func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDVar(r, "id")
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid id"))
		return
	}
	isAdmin := AuthIsAdminFromContext(r.Context())
	if !isAdmin && id != AuthUserIDFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("forbidden"))
		return
	}

	type userPatch struct {
		DisplayName *string      `json:"displayName"`
		Username    *string      `json:"username"`
		Email       *string      `json:"email"`
		Role        *UserRole    `json:"role"`
		Permissions *Permissions `json:"permissions"`
		Preferences *Preferences `json:"preferences"`
	}
	var req struct {
		Password string    `json:"password"`
		User     userPatch `json:"user"`
		Data     userPatch `json:"data"` // Frontend might send it wrapped in "data"
	}
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}

	// Unify req.User and req.Data
	userData := req.User
	if userData.DisplayName == nil && userData.Username == nil && userData.Email == nil && userData.Role == nil && userData.Permissions == nil && userData.Preferences == nil {
		userData = req.Data
	}

	u, err := a.userSvc.GetByID(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}

	if userData.DisplayName != nil {
		u.DisplayName = *userData.DisplayName
	}
	if isAdmin && userData.Username != nil {
		u.Username = *userData.Username
	}
	if isAdmin && userData.Email != nil {
		u.Email = *userData.Email
	}

	if userData.Preferences != nil {
		u.Preferences = *userData.Preferences
	}
	if isAdmin && userData.Role != nil {
		u.Role = *userData.Role
	}
	if isAdmin && userData.Permissions != nil {
		u.Permissions = *userData.Permissions
	}
	if req.Password != "" {
		settings, _ := a.settingsSvc.Get(r.Context())
		if settings != nil && len(req.Password) < settings.MinimumPasswordLength {
			WriteJSON(w, http.StatusBadRequest, ErrorResponse("password too short"))
			return
		}
		hash, err := hashPassword(req.Password)
		if err != nil {
			WriteErr(w, WrapError(ErrInternal, err, "cannot hash password"))
			return
		}
		u.PasswordHash = hash
	}

	if err := a.userSvc.UpdateUser(r.Context(), u); err != nil {
		WriteErr(w, err)
		return
	}
	u.Sanitize()
	WriteJSON(w, http.StatusOK, u.ToFrontend())
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
		return
	}
	id, err := parseIDVar(r, "id")
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid id"))
		return
	}
	if err := a.userSvc.DeleteUser(r.Context(), id, a.sessionStore, a.cleanupUserData); err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Storage handlers ──────────────────────────────────────────────────────────

func (a *App) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	filePath := mux.Vars(r)["path"]
	if filePath == "" {
		filePath = r.URL.Query().Get("path")
	}
	if filePath == "" {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("path is required"))
		return
	}

	// Detect directory creation intent (trailing slash in URL or empty body with a hint?)
	// Frontend uses a trailing slash to signal directory creation.
	isDir := strings.HasSuffix(r.URL.Path, "/") || strings.HasSuffix(filePath, "/")
	filePath = normalizePath(filePath)

	var file *File
	var err error
	if isDir {
		file, err = a.storageSvc.CreateDir(r.Context(), uid, filePath)
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodySize)
		file, err = a.storageSvc.WriteFile(r.Context(), uid, filePath, r.Body)
	}

	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, file)
}

func (a *App) handleFileList(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	filePath := mux.Vars(r)["path"]
	if filePath == "" {
		filePath = r.URL.Query().Get("path")
	}
	filePath = normalizePath(filePath)

	// Fetch directory metadata
	var self *File
	var err error
	if filePath == "/" {
		self = &File{Path: "/", Name: "", Type: EntryDir, ModifiedAt: time.Now().UTC()}
	} else {
		self, err = a.storageSvc.GetFileByPath(r.Context(), uid, filePath)
		if err != nil {
			WriteErr(w, err)
			return
		}
	}

	items, err := a.storageSvc.ListByPath(r.Context(), uid, filePath)
	if err != nil {
		WriteErr(w, err)
		return
	}

	res := self.ToFrontend()
	res["sorting"] = map[string]any{
		"by":  "name",
		"asc": true,
	}
	frontendItems := make([]map[string]any, 0, len(items))
	for _, it := range items {
		frontendItems = append(frontendItems, it.File.ToFrontend())
	}
	res["items"] = frontendItems
	numDirs := 0
	numFiles := 0
	for _, it := range items {
		if it.Type == EntryDir {
			numDirs++
		} else {
			numFiles++
		}
	}
	res["numDirs"] = numDirs
	res["numFiles"] = numFiles

	WriteJSON(w, http.StatusOK, res)
}

func (a *App) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	filePath := mux.Vars(r)["path"]
	if filePath == "" {
		filePath = r.URL.Query().Get("path")
	}
	filePath = normalizePath(filePath)
	f, err := a.storageSvc.GetFileByPath(r.Context(), uid, filePath)
	if err != nil {
		WriteErr(w, err)
		return
	}
	handled, err := RunDeletionHooks(r.Context(), uid, f.Path, f.Type == EntryDir, f.Size)
	if err != nil {
		WriteErr(w, err)
		return
	}
	if handled {
		if err := a.storageSvc.DeleteFileRecord(r.Context(), uid, f.ID); err != nil {
			WriteErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.storageSvc.DeleteFile(r.Context(), uid, f.ID); err != nil {
		WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleFileRename(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	filePath := mux.Vars(r)["path"]
	filePath = normalizePath(filePath)
	f, err := a.storageSvc.GetFileByPath(r.Context(), uid, filePath)
	if err != nil {
		WriteErr(w, err)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Destination string `json:"destination"`
		Action      string `json:"action"` // rename, move, copy
	}

	// Try to decode JSON body first
	if decodeErr := DecodeJSON(r, &req); decodeErr != nil && !errors.Is(decodeErr, io.EOF) {
		slog.Debug("rename request body decode failed, fallback to query", "err", decodeErr)
	}

	// Override with query parameters if present (common in frontend calls)
	if a := r.URL.Query().Get("action"); a != "" {
		req.Action = a
	}
	if d := r.URL.Query().Get("destination"); d != "" {
		req.Destination = d
	}
	if n := r.URL.Query().Get("name"); n != "" {
		req.Name = n
	}

	dest := req.Destination
	if dest == "" && req.Name != "" {
		dest = path.Join(path.Dir(f.Path), req.Name)
	}
	if dest == "" {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("destination or name required"))
		return
	}

	var result *File
	if req.Action == "copy" {
		result, err = a.storageSvc.CopyFile(r.Context(), uid, f.ID, dest)
	} else {
		// Default to rename/move
		result, err = a.storageSvc.RenameFile(r.Context(), uid, f.ID, dest)
	}

	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func (a *App) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	filePath := mux.Vars(r)["path"]
	if filePath == "" {
		relPath := strings.TrimPrefix(r.URL.Path, a.Config.Server.BaseURL)
		filePath = strings.TrimPrefix(relPath, "/api/raw/")
	}
	filePath = normalizePath(filePath)
	f, err := a.storageSvc.GetFileByPath(r.Context(), uid, filePath)
	if err != nil {
		WriteErr(w, err)
		return
	}
	rc, _, err := a.storageSvc.OpenFile(r.Context(), uid, f.ID)
	if err != nil {
		WriteErr(w, err)
		return
	}
	defer func() { _ = rc.Close() }()
	if f.Media.MIME != "" {
		w.Header().Set("Content-Type", f.Media.MIME)
	}
	disp := mime.FormatMediaType("inline", map[string]string{"filename": f.Name})
	w.Header().Set("Content-Disposition", disp)
	_, _ = io.Copy(w, rc)
}

func (a *App) handleFilePreview(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	vars := mux.Vars(r)
	size := vars["size"]
	filePath := vars["path"]
	if filePath == "" {
		relPath := strings.TrimPrefix(r.URL.Path, a.Config.Server.BaseURL)
		parts := strings.SplitN(strings.TrimPrefix(relPath, "/api/preview/"), "/", 2)
		if len(parts) == 2 {
			size = parts[0]
			filePath = parts[1]
		}
	}
	filePath = normalizePath(filePath)
	width, height := 320, 320
	if size == "big" {
		width, height = 1024, 1024
	}

	f, err := a.storageSvc.GetFileByPath(r.Context(), uid, filePath)
	if err != nil {
		WriteErr(w, err)
		return
	}

	preview, mime, err := a.storageSvc.RenderPreview(r.Context(), uid, f.ID, width, height)
	if err != nil {
		WriteErr(w, err)
		return
	}
	w.Header().Set("Content-Type", mime)
	_, _ = w.Write(preview)
}

// ── Task handlers ─────────────────────────────────────────────────────────────

func (a *App) handleTaskSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	id, err := a.taskSvc.SubmitRegistered(r.Context(), req.Name, uid)
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"id": id})
}

func (a *App) handleTaskList(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	items, err := a.taskSvc.ListByUser(r.Context(), uid)
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, items)
}

func (a *App) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}
	id := mux.Vars(r)["id"]
	task, err := a.taskSvc.GetByID(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}
	if task.UserID != uid && !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("can only cancel your own tasks"))
		return
	}
	if err := a.taskSvc.Cancel(id); err != nil {
		WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTaskEvents(w http.ResponseWriter, r *http.Request) {
	uid := AuthUserIDFromContext(r.Context())
	if uid == 0 {
		WriteJSON(w, http.StatusUnauthorized, ErrorResponse("unauthorized"))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	ch := a.taskSvc.Subscribe(uid)
	defer a.taskSvc.Unsubscribe(ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial empty event to establish connection
	_, _ = fmt.Fprintf(w, ": ok\n\n")
	flusher.Flush()

	for {
		select {
		case t, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(t)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		case <-time.After(25 * time.Second):
			// Heartbeat keeps intermediaries from reaping idle SSE streams.
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ── Settings handlers ─────────────────────────────────────────────────────────

func (a *App) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handleSettingsGet hit")
	if !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
		return
	}
	s, err := a.settingsSvc.Get(r.Context())
	if err != nil {
		WriteErr(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, s)
}

func (a *App) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	if !AuthIsAdminFromContext(r.Context()) {
		WriteJSON(w, http.StatusForbidden, ErrorResponse("admin only"))
		return
	}
	in := new(Settings)
	if err := DecodeJSON(r, in); err != nil {
		WriteJSON(w, http.StatusBadRequest, ErrorResponse("invalid json"))
		return
	}
	if err := a.settingsSvc.Save(r.Context(), in); err != nil {
		WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseIDVar(r *http.Request, _ string) (uint64, error) { //nolint:unparam // Handler callers always use the "id" route variable.
	return strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
}

// ── Index handler ────────────────────────────────────────────────────────────

type indexData struct {
	Name      string
	StaticURL string
	BaseURL   string
	Color     string
	Nonce     string
	Json      template.JS
	CSS       bool
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if www.PublicFS == nil {
		http.Error(w, "frontend not found", http.StatusNotFound)
		return
	}
	data, err := fs.ReadFile(www.PublicFS, "index.html")
	if err != nil {
		slog.Error("failed to read index.html", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	htmlContent := string(data)
	if a.Config.Server.BaseURL != "" {
		// Prepend BaseURL to absolute asset paths generated by Vite.
		htmlContent = strings.ReplaceAll(htmlContent, "src=\"/assets/", "src=\"[{[ .StaticURL ]}]/assets/")
		htmlContent = strings.ReplaceAll(htmlContent, "href=\"/assets/", "href=\"[{[ .StaticURL ]}]/assets/")
		htmlContent = strings.ReplaceAll(htmlContent, "src=\"/img/", "src=\"[{[ .StaticURL ]}]/img/")
		htmlContent = strings.ReplaceAll(htmlContent, "href=\"/img/", "href=\"[{[ .StaticURL ]}]/img/")
		htmlContent = strings.ReplaceAll(htmlContent, "href=\"/manifest.json\"", "href=\"[{[ .StaticURL ]}]/manifest.json\"")
	}

	settings, _ := a.settingsSvc.Get(r.Context())
	if settings == nil {
		settings = &Settings{Branding: SettingsBranding{Name: "Abyss"}}
	}

	// Prepare window.Abyss JSON to drive frontend configuration
	abyssMap := map[string]any{
		"Name":                  settings.Branding.Name,
		"BaseURL":               a.Config.Server.BaseURL,
		"StaticURL":             a.Config.Server.BaseURL,
		"Version":               "0.0.1",
		"Theme":                 settings.Branding.Theme,
		"Signup":                settings.Signup,
		"NoAuth":                false,
		"DisableExternal":       settings.Branding.DisableExternal,
		"DisableUsedPercentage": settings.Branding.DisableUsedPercentage,
		"AuthMethod":            "local",
		"LoginPage":             true,
		"LogoutPage":            "",
		"EnableThumbs":          true,
		"ResizePreview":         true,
		"HideLoginButton":       settings.HideLoginButton,
		"Demo":                  a.Config.Demo,
		"TusSettings":           settings.Tus,
	}
	// Only expose demo credentials when demo mode is actually enabled;
	// otherwise they would leak into the anonymously served index.html.
	if a.Config.Demo {
		abyssMap["DemoEmail"] = a.Config.DemoEmail
		abyssMap["DemoPassword"] = a.Config.DemoPassword
	}
	abyssJSON, _ := json.Marshal(abyssMap)

	id := indexData{
		Name:      settings.Branding.Name,
		StaticURL: a.Config.Server.BaseURL,
		BaseURL:   a.Config.Server.BaseURL + "/",
		Nonce:     "",
		Json:      template.JS(abyssJSON), //nolint:gosec // abyssJSON is server-generated JSON marshaled from trusted values.
	}

	// Parse once and cache: htmlContent only depends on the (static)
	// BaseURL; dynamic values are injected at Execute time.
	if a.indexTmpl == nil {
		t, parseErr := template.New("index").Delims("[{[", "]}]").Parse(htmlContent)
		if parseErr != nil {
			slog.Error("failed to parse index template", "err", parseErr)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		a.indexTmpl = t
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.indexTmpl.Execute(w, id); err != nil {
		slog.Error("failed to execute index template", "err", err)
	}
}

func (a *App) handleGCStatus(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"running": false, "history": []any{}})
}

func (a *App) handleGCRun(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"deletedSize": 0, "deletedCount": 0})
}

func (a *App) handleGetMigrationStatus(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"status": "idle"})
}

func (a *App) handleStartMigration(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"status": "started"})
}

func (a *App) handlePreflightMigration(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"possible": true})
}
