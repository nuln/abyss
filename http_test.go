package abyss

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── test app factory ──────────────────────────────────────────────────────────

func newTestApp(t *testing.T) *App {
	t.Helper()
	jwtSecret := make([]byte, 32)
	users := newMemUserStore()
	sessions := newMemSessionStore()
	fileStore := newMemFileStore()
	settingsSvc := newSettingsService(&memSettingsStore{})

	engine := newMemEngine()
	// Pre-populate test user 1
	_ = users.Create(t.Context(), &User{ID: 1, UUID: "test-user-1-uuid", Email: "system-test@example.com"})
	storageSvc := newStorageService(fileStore, users, settingsSvc, t.TempDir())
	storageSvc.engines[1] = engine // map user 1 to mem engine

	userSvc := &userService{store: users}
	authSvc := newAuthService(users, sessions, jwtSecret, time.Hour, 7*24*time.Hour, true)
	taskSvc := newTaskService(newMemTaskStore(), newScheduler())
	app := &App{
		userSvc:      userSvc,
		authSvc:      authSvc,
		storageSvc:   storageSvc,
		taskSvc:      taskSvc,
		settingsSvc:  settingsSvc,
		sessionStore: sessions,
		Router:       mux.NewRouter(),
	}

	// For testing, enable signup and set a short minimum password length
	_ = settingsSvc.Save(t.Context(), &Settings{
		Signup:                true,
		MinimumPasswordLength: 1,
		Defaults: SettingsDefaults{
			Locale: "auto",
			Theme:  "auto",
		},
		Branding: SettingsBranding{
			Theme: "auto",
		},
		StorageType: "path",
	})

	registerAllRoutes(app.Router, app)
	return app
}

func doPostRequest(t *testing.T, app *App, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	return rr
}

func doAuthRequest(t *testing.T, app *App, method, path string, body any, userID uint64, isAdmin bool) *httptest.ResponseRecorder {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	// Inject a valid JWT
	u := &User{ID: userID, Role: RoleUser}
	if isAdmin {
		u.Role = RoleAdmin
	}
	token, err := app.authSvc.signJWT(u)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	return rr
}

// ── context helpers tests ─────────────────────────────────────────────────────

func TestWithAuthContext(t *testing.T) {
	ctx := withAuthUserID(t.Context(), 42)
	ctx = withAuthIsAdmin(ctx, true)
	assert.Equal(t, uint64(42), AuthUserIDFromContext(ctx))
	assert.True(t, AuthIsAdminFromContext(ctx))
}

func TestAuthContext_Defaults(t *testing.T) {
	assert.Zero(t, AuthUserIDFromContext(t.Context()))
	assert.False(t, AuthIsAdminFromContext(t.Context()))
}

// ── ErrorResponse / WriteErr ──────────────────────────────────────────────────

func TestErrorResponse(t *testing.T) {
	e := ErrorResponse("something broke")
	assert.Equal(t, "something broke", e["error"])
}

func TestWriteErr_AppError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteErr(rr, ErrNotFound)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestWriteErr_GenericError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteErr(rr, ErrInternal)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestWriteErr_EmptyGenericError_DoesNotPanic(t *testing.T) {
	rr := httptest.NewRecorder()
	assert.NotPanics(t, func() {
		WriteErr(rr, errors.New(""))
	})
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ── Identity HTTP handlers ────────────────────────────────────────────────────

func TestHandleRegister_Success(t *testing.T) {
	app := newTestApp(t)
	rr := doPostRequest(t, app, "/api/signup", map[string]any{
		"email":    "new@example.com",
		"username": "newuser",
		"password": "secret",
	})
	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestHandleSetupStatus(t *testing.T) {
	app := newTestApp(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", http.NoBody)
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"initialized":true`)
}

func TestHandleRegister_DuplicateEmail(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "dup@example.com", "username": "u1", "password": "pass",
	})
	rr := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "dup@example.com", "username": "u2", "password": "pass",
	})
	assert.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandleRegister_BadJSON(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/signup", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleLogin_Success(t *testing.T) {
	app := newTestApp(t)
	// register first
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "test@example.com", "username": "testuser", "password": "password",
	})
	require.Equal(t, http.StatusCreated, rrReg.Code)
	rr := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "test@example.com", "password": "password",
	})
	assert.Equal(t, http.StatusOK, rr.Code)
	var res struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.NotEmpty(t, res.Data.AccessToken)
	assert.NotEmpty(t, res.Data.RefreshToken)
}

func TestHandleLogin_Username(t *testing.T) {
	app := newTestApp(t)
	// register first
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "test@example.com", "username": "testuser", "password": "password",
	})
	require.Equal(t, http.StatusCreated, rrReg.Code)

	// Login with username instead of email
	rr := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "testuser", "password": "password",
	})
	assert.Equal(t, http.StatusOK, rr.Code)
	var res struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.NotEmpty(t, res.Data.AccessToken)
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "wp@example.com", "username": "wpu", "password": "correct",
	})
	rr := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "wp@example.com", "password": "wrong",
	})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleRefresh(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "ref@example.com", "username": "refu", "password": "pw",
	})
	assert.Equal(t, http.StatusCreated, rrReg.Code)
	loginRR := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "ref@example.com", "password": "pw",
	})
	var loginRes struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(loginRR.Body.Bytes(), &loginRes))

	rr := doPostRequest(t, app, "/api/auth/refresh", map[string]any{
		"refreshToken": loginRes.Data.RefreshToken,
	})
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleRefresh_InvalidToken(t *testing.T) {
	app := newTestApp(t)
	rr := doPostRequest(t, app, "/api/auth/refresh", map[string]any{
		"refreshToken": "bogus",
	})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleLogout(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "logout@example.com", "username": "lo", "password": "pw",
	})
	loginRR := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "logout@example.com", "password": "pw",
	})
	var loginRes struct {
		Data AuthResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(loginRR.Body.Bytes(), &loginRes))

	rr := doAuthRequest(t, app, http.MethodPost, "/api/auth/logout", map[string]any{
		"refreshToken": loginRes.Data.RefreshToken,
	}, loginRes.Data.UserID, false)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleRevokeSession_RejectsOtherUsersSession(t *testing.T) {
	app := newTestApp(t)
	require.NoError(t, app.sessionStore.(*memSessionStore).Save(t.Context(), &RefreshToken{
		ID:        "other-session",
		UserID:    2,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	rr := doAuthRequest(t, app, http.MethodDelete, "/api/auth/sessions/other-session", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleRevokeSession_AdminCanRevokeOtherUsersSession(t *testing.T) {
	app := newTestApp(t)
	require.NoError(t, app.sessionStore.(*memSessionStore).Save(t.Context(), &RefreshToken{
		ID:        "other-session",
		UserID:    2,
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	rr := doAuthRequest(t, app, http.MethodDelete, "/api/auth/sessions/other-session", nil, 1, true)
	assert.Equal(t, http.StatusOK, rr.Code)
	_, err := app.sessionStore.GetByID(t.Context(), "other-session")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHandleMe_Authenticated(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "me@example.com", "username": "meuser", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	rr := doAuthRequest(t, app, http.MethodGet, "/api/me", nil, regRes.Data.ID, false)
	assert.Equal(t, http.StatusOK, rr.Code)
	var got struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, regRes.Data.ID, uint64(got.Data["id"].(float64)))
}

func TestHandleMe_Unauthenticated(t *testing.T) {
	app := newTestApp(t)
	// Make request without Auth header — middleware should block
	req := httptest.NewRequest(http.MethodGet, "/api/me", http.NoBody)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthMiddleware_QueryTokenDisabled(t *testing.T) {
	app := newTestApp(t)
	app.authSvc.allowQueryToken = false

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/me?auth="+tok, http.NoBody)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleUpdateMe(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "upd@example.com", "username": "updu", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	rr := doAuthRequest(t, app, http.MethodPut, "/api/me", map[string]any{
		"displayName": "Updated",
	}, regRes.Data.ID, false)
	assert.Equal(t, http.StatusOK, rr.Code)
	var got struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, "Updated", got.Data["displayName"])
}

func TestHandleListUsers_AdminOnly(t *testing.T) {
	app := newTestApp(t)

	// non-admin gets 403
	rr := doAuthRequest(t, app, http.MethodGet, "/api/users", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	// admin gets 200
	rr = doAuthRequest(t, app, http.MethodGet, "/api/users", nil, 1, true)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleGetUser_AdminOnly(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "gu@example.com", "username": "guu", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	rr := doAuthRequest(t, app, http.MethodGet, "/api/users/999", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	rr = doAuthRequest(t, app, http.MethodGet, "/api/users/999", nil, 1, true)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleDeleteUser_AdminOnly(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "du@example.com", "username": "duu", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	loginRR := doPostRequest(t, app, "/api/auth/login", map[string]any{
		"email": "du@example.com", "password": "pw",
	})
	assert.Equal(t, http.StatusOK, loginRR.Code)
	sessions := app.sessionStore.(*memSessionStore)
	assert.Len(t, sessions.sessions, 1)

	// non-admin
	rr := doAuthRequest(t, app, http.MethodDelete, "/api/users/1", nil, 99, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	// admin deletes existing user
	deletePath := "/api/users/" + uintToStr(regRes.Data.ID)
	rr = doAuthRequest(t, app, http.MethodDelete, deletePath, nil, 99, true)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, sessions.sessions)
}

// ── Settings HTTP handlers ────────────────────────────────────────────────────

func TestHandleSettingsGet_Admin(t *testing.T) {
	app := newTestApp(t)
	// First save some settings via admin
	rr := doAuthRequest(t, app, http.MethodPut, "/api/settings", &Settings{
		Branding: SettingsBranding{Name: "TestApp"},
	}, 1, true)
	assert.Equal(t, http.StatusNoContent, rr.Code)

	rr2 := doAuthRequest(t, app, http.MethodGet, "/api/settings", nil, 1, true)
	assert.Equal(t, http.StatusOK, rr2.Code)
	var s struct {
		Data Settings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &s))
	assert.Equal(t, "TestApp", s.Data.Branding.Name)
}

func TestHandleSettingsGet_NonAdmin_Forbidden(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/settings", nil, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleSettingsSave_Admin(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPut, "/api/settings", map[string]any{
		"branding": map[string]string{"name": "My Abyss", "theme": "dark"},
	}, 1, true)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandleSettingsSave_NonAdmin_Forbidden(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPut, "/api/settings", map[string]any{}, 1, false)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

// ── Task HTTP handlers ────────────────────────────────────────────────────────

func TestHandleTaskList(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/tasks", nil, 1, false)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleTaskSubmit_UnregisteredRunner(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPost, "/api/tasks", map[string]any{
		"name": "no-such-runner",
	}, 1, false)
	// Runner not registered → should fail with some error (500 or 400)
	assert.GreaterOrEqual(t, rr.Code, 400)
}

func TestHandleTaskCancel_NotFound(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodPost, "/api/tasks/nonexistent-id/cancel", nil, 1, false)
	// Cancel of unknown task returns an error (scheduler doesn't know it)
	assert.GreaterOrEqual(t, rr.Code, 400)
}

// ── Storage HTTP handlers ─────────────────────────────────────────────────────

func TestHandleFileList(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/resources/?path=/", nil, 1, false)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleFileUpload(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "uploader@example.com", "username": "uploader", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))
	app.storageSvc.engines[regRes.Data.ID] = newMemEngine()

	req := httptest.NewRequest(http.MethodPost, "/api/resources/?path=/hello.txt", bytes.NewBufferString("file contents"))
	req.Header.Set("Content-Type", "application/octet-stream")
	tok, _ := app.authSvc.signJWT(&User{ID: regRes.Data.ID, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)
	var f struct {
		Data File `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &f))
	assert.Equal(t, "/hello.txt", f.Data.Path)
}

func TestHandleMkdir(t *testing.T) {
	app := newTestApp(t)
	rrReg := doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "mkdir@example.com", "username": "mkdiruser", "password": "pw",
	})
	var regRes struct {
		Data User `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rrReg.Body.Bytes(), &regRes))

	// POST to path with trailing slash
	req := httptest.NewRequest(http.MethodPost, "/api/resources/my_folder/", http.NoBody)
	tok, _ := app.authSvc.signJWT(&User{ID: regRes.Data.ID, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var res struct {
		Data File `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.Equal(t, EntryDir, res.Data.Type)
	assert.Equal(t, "/my_folder", res.Data.Path)
}

func TestHandleFileUpload_MissingPath(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/resources/", bytes.NewBufferString("data"))
	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleFileGetByID_NotFound(t *testing.T) {
	app := newTestApp(t)
	rr := doAuthRequest(t, app, http.MethodGet, "/api/resources/9999", nil, 1, false)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func uintToStr(n uint64) string {
	return strconv.FormatUint(n, 10)
}
func TestHandleFileMove(t *testing.T) {
	app := newTestApp(t)
	// Create file
	doAuthRequest(t, app, http.MethodPost, "/api/resources/old.txt", bytes.NewBufferString("data"), 1, false)

	// PATCH with query params
	req := httptest.NewRequest(http.MethodPatch, "/api/resources/old.txt?action=rename&destination=/new.txt", http.NoBody)
	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var res struct {
		Data File `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.Equal(t, "/new.txt", res.Data.Path)
}

func TestHandleFileCopy(t *testing.T) {
	app := newTestApp(t)
	doAuthRequest(t, app, http.MethodPost, "/api/resources/src.txt", bytes.NewBufferString("data"), 1, false)

	req := httptest.NewRequest(http.MethodPatch, "/api/resources/src.txt?action=copy&destination=/dst.txt", http.NoBody)
	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleFileDelete_NormalizesPath(t *testing.T) {
	app := newTestApp(t)
	app.storageSvc.engines[1] = newMemEngine()
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/docs/a.txt", bytes.NewBufferString("x"))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodDelete, "/api/resources/?path=/docs/../docs//a.txt", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandleFileDownload_ContentDispositionEscaped(t *testing.T) {
	app := newTestApp(t)
	engine := newMemEngine()
	app.storageSvc.engines[1] = engine
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/evil\"name.txt", bytes.NewBufferString("x"))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/raw/evil\"name.txt", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	cd := rr.Header().Get("Content-Disposition")
	assert.Contains(t, cd, "inline")
	assert.NotContains(t, cd, "\n")
	assert.NotContains(t, cd, "\r")
	assert.Equal(t, "x", rr.Body.String())
}

func TestHandleFileDownload_ContentMatchesUploadedFile(t *testing.T) {
	app := newTestApp(t)
	engine := newMemEngine()
	app.storageSvc.engines[1] = engine
	payload := "download-payload"
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/dl.txt", bytes.NewBufferString(payload))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/raw/dl.txt", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, payload, rr.Body.String())
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "filename")
}

func TestHandleFilePreview_ReturnsJPEG(t *testing.T) {
	app := newTestApp(t)
	engine := newMemEngine()
	app.storageSvc.engines[1] = engine

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	var src bytes.Buffer
	require.NoError(t, png.Encode(&src, img))
	_, err := app.storageSvc.WriteFile(t.Context(), 1, "/preview.png", bytes.NewReader(src.Bytes()))
	require.NoError(t, err)

	tok, _ := app.authSvc.signJWT(&User{ID: 1, Role: RoleUser})
	req := httptest.NewRequest(http.MethodGet, "/api/preview/small/preview.png", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	app.Router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
}

func TestHandleUpdateUser(t *testing.T) {
	app := newTestApp(t)
	// Signup user 1
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "u1@e.com", "username": "u1", "password": "pw",
	})
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "u2@e.com", "username": "u2", "password": "pw",
	})

	// admin updates user 2
	rrAdmin := doAuthRequest(t, app, http.MethodPut, "/api/users/2", map[string]any{
		"data": map[string]any{"displayName": "Admin Updated"},
	}, 99, true)
	assert.Equal(t, http.StatusOK, rrAdmin.Code)

	// Update user 1 as user 1
	rr := doAuthRequest(t, app, http.MethodPut, "/api/users/1", map[string]any{
		"data": map[string]any{"displayName": "New Name"},
	}, 1, false)
	assert.Equal(t, http.StatusOK, rr.Code)

	var res struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &res))
	assert.Equal(t, "New Name", res.Data["displayName"])
}

func TestHandleUpdateUser_SelfCannotEscalateRoleOrEditEmail(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "self@e.com", "username": "self", "password": "pw",
	})

	rr := doAuthRequest(t, app, http.MethodPut, "/api/users/2", map[string]any{
		"data": map[string]any{
			"displayName": "Safe Name",
			"email":       "attacker@example.com",
			"role":        "admin",
			"permissions": map[string]any{"admin": true},
		},
	}, 2, false)
	assert.Equal(t, http.StatusOK, rr.Code)

	u, err := app.userSvc.GetByID(t.Context(), 2)
	require.NoError(t, err)
	assert.Equal(t, "Safe Name", u.DisplayName)
	assert.Equal(t, "self@e.com", u.Email)
	assert.Equal(t, RoleUser, u.Role)
	assert.False(t, u.Permissions.Admin)
}

func TestHandleUpdateUser_AdminCanUpdateRoleAndPermissions(t *testing.T) {
	app := newTestApp(t)
	doPostRequest(t, app, "/api/signup", map[string]any{
		"email": "managed@e.com", "username": "managed", "password": "pw",
	})

	rr := doAuthRequest(t, app, http.MethodPut, "/api/users/2", map[string]any{
		"data": map[string]any{
			"role": "admin",
			"permissions": map[string]any{
				"admin":  true,
				"create": true,
				"rename": true,
			},
		},
	}, 1, true)
	assert.Equal(t, http.StatusOK, rr.Code)

	u, err := app.userSvc.GetByID(t.Context(), 2)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, u.Role)
	assert.True(t, u.Permissions.Admin)
	assert.True(t, u.Permissions.Create)
	assert.True(t, u.Permissions.Rename)
}

func TestDecodeJSON_RejectsOversizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(bytes.Repeat([]byte("a"), int(maxJSONBodySize+1))))
	var payload map[string]any
	err := DecodeJSON(req, &payload)
	assert.Error(t, err)
}
