package abyss

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── api ──
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
	authSvc := newAuthService(users, sessions, jwtSecret, jwtSecret, time.Hour, 7*24*time.Hour, true)
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

// ── plugin ──
// ── Shared stubs ──────────────────────────────────────────────────────────────

type stubPlugin struct {
	slug string
	deps []string
	typ  PluginType
}

func (s *stubPlugin) Info() PluginInfo {
	pt := s.typ
	if pt == "" {
		pt = TypeFree
	}
	return PluginInfo{
		SlugName:     s.slug,
		Type:         pt,
		Dependencies: s.deps,
	}
}

func newStub(slug string, deps ...string) Base {
	return &stubPlugin{slug: slug, deps: deps}
}

func newPaidStub(slug string, deps ...string) Base {
	return &stubPlugin{slug: slug, typ: TypePaid, deps: deps}
}

// ── interfaces_test ───────────────────────────────────────────────────────────

type stubAuthenticator struct {
	slug    string
	methods []PluginAuthMethod
	login   *PluginAuthResult
	err     error
}

func (s *stubAuthenticator) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubAuthenticator) AuthMethods() []PluginAuthMethod {
	return s.methods
}

func (s *stubAuthenticator) Authenticate(_ string, _ map[string]interface{}) (uint64, error) {
	if s.login != nil {
		return 1, s.err
	}
	return 0, s.err
}

func (s *stubAuthenticator) OnLoginSuccess(_ uint64, _ *http.Request) (*PluginAuthResult, error) {
	return s.login, s.err
}

func (s *stubAuthenticator) VerifyMFA(_ uint64, _ string, _ map[string]interface{}) (bool, error) {
	return s.err == nil, s.err
}

func (s *stubAuthenticator) OnRegisterSuccess(_ uint64, _ *http.Request) error {
	return s.err
}

type stubNotification struct {
	slug string
	sent *[]NotificationMessage
	err  error
}

func (s *stubNotification) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubNotification) Send(msg NotificationMessage) error {
	if s.sent != nil {
		*s.sent = append(*s.sent, msg)
	}
	return s.err
}

type stubDeletionHook struct {
	slug    string
	handled bool
	err     error
}

func (s *stubDeletionHook) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubDeletionHook) OnDelete(_ context.Context, _ uint64, _ string, _ bool, _ int64) (bool, error) {
	return s.handled, s.err
}

type stubConfigPlugin struct {
	slug   string
	config []byte
	inits  int
}

func (s *stubConfigPlugin) Info() PluginInfo {
	return PluginInfo{SlugName: s.slug, Type: TypeFree}
}

func (s *stubConfigPlugin) Init(_ *StartupContext) error {
	s.inits++
	return nil
}

func (s *stubConfigPlugin) Stop(_ context.Context) error { return nil }

func (s *stubConfigPlugin) ConfigFields() []ConfigField { return nil }

func (s *stubConfigPlugin) ConfigReceiver(config []byte) error {
	s.config = append([]byte(nil), config...)
	return nil
}

func TestGetAuthMethods_ReturnsEmptyWhenNothingRegistered(t *testing.T) {
	oldCall := CallAuthenticator
	t.Cleanup(func() {
		CallAuthenticator = oldCall
	})

	call, _, _, _ := MakePlugin[Authenticator](false)
	CallAuthenticator = call

	assert.Empty(t, GetAuthMethods())
}

func TestGetAuthMethods_AggregatesAcrossAuthenticators(t *testing.T) {
	oldCall := CallAuthenticator
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallAuthenticator = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Authenticator](false)
	register(&stubAuthenticator{slug: "passkey", methods: []PluginAuthMethod{{SlugName: "passkey", Name: "Passkey"}}})
	register(&stubAuthenticator{slug: "otp", methods: []PluginAuthMethod{{SlugName: "otp", Name: "OTP"}}})
	CallAuthenticator = call

	methods := GetAuthMethods()
	require.Len(t, methods, 2)
	assert.Equal(t, []string{"passkey", "otp"}, []string{methods[0].SlugName, methods[1].SlugName})
}

func TestSendNotification_DispatchesToAllRegisteredNotifications(t *testing.T) {
	oldCall := CallNotification
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallNotification = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Notification](false)
	var sentA []NotificationMessage
	var sentB []NotificationMessage
	register(&stubNotification{slug: "smtp", sent: &sentA})
	register(&stubNotification{slug: "webhook", sent: &sentB})
	CallNotification = call

	msg := NotificationMessage{Type: NotifyUserCreated, Recipient: "u1", Subject: "subject", Body: "body"}
	SendNotification(msg)

	require.Len(t, sentA, 1)
	require.Len(t, sentB, 1)
	assert.Equal(t, msg, sentA[0])
	assert.Equal(t, msg, sentB[0])
}

func TestSendNotification_SwallowsPluginError(t *testing.T) {
	oldCall := CallNotification
	oldStatusManager := StatusManager
	t.Cleanup(func() {
		CallNotification = oldCall
		StatusManager = oldStatusManager
	})

	StatusManager = nil
	call, _, _, register := MakePlugin[Notification](false)
	var sent []NotificationMessage
	register(&stubNotification{slug: "smtp", sent: &sent, err: errors.New("send failed")})
	CallNotification = call

	assert.NotPanics(t, func() {
		SendNotification(NotificationMessage{Type: NotifyPasswordReset, Recipient: "u1"})
	})
	require.Len(t, sent, 1)
}

func TestDeletionHook_MakePluginStopsAfterHandled(t *testing.T) {
	call, _, _, register := MakePlugin[DeletionHook](true)
	seen := 0
	register(&stubDeletionHook{slug: "trash", handled: true})
	register(&stubDeletionHook{slug: "audit", handled: false})

	err := call(func(h DeletionHook) error {
		seen++
		handled, err := h.OnDelete(context.Background(), 1, "/tmp/file", false, 123)
		if err != nil {
			return err
		}
		if handled {
			return errors.New("handled")
		}
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, 1, seen)
}

func TestMemoryEventBus_Close(t *testing.T) {
	bus := NewMemoryEventBus()
	var calls atomic.Int32

	_ = bus.Subscribe("topic", func(_ interface{}) {
		calls.Add(1)
	})

	bus.Publish("topic", "first")
	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 10*time.Millisecond)

	bus.Close()
	bus.Publish("topic", "second")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), calls.Load())
}

// ── registry_test ─────────────────────────────────────────────────────────────

func TestPlugin_Info_Paid(t *testing.T) {
	p := newPaidStub("paid-plugin", "dep1")
	info := p.Info()
	assert.Equal(t, "paid-plugin", info.SlugName)
	assert.Equal(t, TypePaid, info.Type)
	assert.Contains(t, info.Dependencies, "dep1")
}

func TestSortPlugins_NoDependencies(t *testing.T) {
	plugins := []Base{
		newStub("a"),
		newStub("b"),
		newStub("c"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	assert.Len(t, sorted, 3)
	slugs := make(map[string]bool)
	for _, p := range sorted {
		slugs[p.Info().SlugName] = true
	}
	assert.True(t, slugs["a"])
	assert.True(t, slugs["b"])
	assert.True(t, slugs["c"])
}

func TestSortPlugins_Simple(t *testing.T) {
	plugins := []Base{
		newStub("b", "a"),
		newStub("a"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	require.Len(t, sorted, 2)

	indexA := -1
	indexB := -1
	for i, p := range sorted {
		switch p.Info().SlugName {
		case "a":
			indexA = i
		case "b":
			indexB = i
		}
	}
	assert.Less(t, indexA, indexB, "plugin a must come before plugin b")
}

func TestSortPlugins_MultiLevel(t *testing.T) {
	plugins := []Base{
		newStub("c", "b"),
		newStub("a"),
		newStub("b", "a"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	require.Len(t, sorted, 3)

	idx := map[string]int{}
	for i, p := range sorted {
		idx[p.Info().SlugName] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["b"], idx["c"])
}

func TestSortPlugins_CircularDependency(t *testing.T) {
	plugins := []Base{
		newStub("a", "b"),
		newStub("b", "a"),
	}
	_, err := SortPlugins(plugins)
	assert.Error(t, err, "circular dependency should be detected")
}

func TestSortPlugins_UnknownDependencyIsIgnored(t *testing.T) {
	plugins := []Base{
		newStub("a"),
		newStub("b", "external"),
	}
	sorted, err := SortPlugins(plugins)
	require.NoError(t, err)
	assert.Len(t, sorted, 2)
}

func TestStack_Register_And_Get(t *testing.T) {
	call, _, get, register := MakePlugin[Base](true)

	pa := newStub("test-reg-a")
	pb := newStub("test-reg-b")
	register(pa)
	register(pb)

	retrieved := get("test-reg-a")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "test-reg-a", retrieved.Info().SlugName)

	count := 0
	err := call(func(_ Base) error {
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestStack_DuplicateRegistrationPanics(t *testing.T) {
	_, _, _, register := MakePlugin[Base](true)

	register(newStub("dup-slug"))
	assert.Panics(t, func() {
		register(newStub("dup-slug"))
	}, "registering the same slug twice should panic")
}

func TestStack_ConcurrentRegister_Safe(t *testing.T) {
	_, _, _, register := MakePlugin[Base](true)

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		slug := "concurrent-" + string(rune('a'+i))
		go func(s string) {
			defer wg.Done()
			register(&stubPlugin{slug: s})
		}(slug)
	}
	wg.Wait()
}

func TestStack_GetNonExistent_ReturnsNil(t *testing.T) {
	_, _, get, _ := MakePlugin[Base](true)
	result := get("does-not-exist")
	var zero Base
	assert.Equal(t, zero, result)
}

// ── manager_test ──────────────────────────────────────────────────────────────

func newTestManager() *statusManager {
	m := &statusManager{status: make(map[string]bool)}
	StatusManager = m
	return m
}

func registerNewStub(slug string, deps ...string) {
	RegisterBase(&stubPlugin{slug: slug, typ: TypeFree, deps: deps})
}

func TestStatusManager_Plugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-free"
	registerNewStub(slug)

	assert.False(t, m.IsEnabled(slug), "plugin should be disabled by default")
}

func TestStatusManager_PaidPlugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-paid"
	RegisterBase(&stubPlugin{slug: slug, typ: TypePaid})

	assert.False(t, m.IsEnabled(slug), "paid (pro) plugin should be disabled by default")
}

func TestStatusManager_UnknownPlugin_DefaultDisabled(t *testing.T) {
	m := newTestManager()
	assert.False(t, m.IsEnabled("totally-unknown"), "fallback for unknown plugin is false")
}

func TestStatusManager_Enable_FreePlugin(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-fp"
	registerNewStub(slug)

	require.NoError(t, m.Enable(slug, true))
	assert.True(t, m.IsEnabled(slug))

	require.NoError(t, m.Enable(slug, false))
	assert.False(t, m.IsEnabled(slug))
}

func TestStatusManager_Enable_DependencyNotActive(t *testing.T) {
	m := newTestManager()
	slugA := t.Name() + "-A"
	slugB := t.Name() + "-B"
	registerNewStub(slugA)
	registerNewStub(slugB, slugA)

	require.NoError(t, m.Enable(slugA, false))

	err := m.Enable(slugB, true)
	assert.Error(t, err, "should fail when dependency is disabled")
}

func TestStatusManager_SetStatuses(t *testing.T) {
	m := newTestManager()
	slugA := t.Name() + "-a"
	slugB := t.Name() + "-b"
	registerNewStub(slugA)
	registerNewStub(slugB)

	m.SetStatuses(map[string]bool{
		slugA: true,
		slugB: false,
	})

	assert.True(t, m.IsEnabled(slugA))
	assert.False(t, m.IsEnabled(slugB))
}

func TestStatusManager_PersistenceHook_Called(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-hook"
	registerNewStub(slug)

	called := false
	m.SetPersistenceHook(func(name string, enabled bool) error {
		if name == slug {
			called = true
		}
		return nil
	})

	require.NoError(t, m.Enable(slug, true))
	assert.True(t, called, "persistence hook should have been called")
}

func TestStatusManager_PersistenceHook_Error_PropagatesError(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-hookerr"
	registerNewStub(slug)

	m.SetPersistenceHook(func(_ string, _ bool) error {
		return errors.New("storage failure")
	})

	err := m.Enable(slug, true)
	assert.Error(t, err)
}

func TestStatusManager_IsEnabled_RaceCondition(t *testing.T) {
	m := newTestManager()
	slug := t.Name() + "-race"
	registerNewStub(slug)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = m.IsEnabled(slug)
		}()
		go func() {
			defer wg.Done()
			_ = m.Enable(slug, true)
		}()
	}
	wg.Wait()
}

// ── skeleton_test ─────────────────────────────────────────────────────────────

func TestSkeleton_Init_SetsCtx(t *testing.T) {
	s := &Skeleton{}
	ctx := &StartupContext{}
	err := s.Init(ctx)
	require.NoError(t, err)
	assert.Equal(t, ctx, s.Ctx)
}

func TestSkeleton_Stop_IsNoop(t *testing.T) {
	s := &Skeleton{Ctx: &StartupContext{}}
	err := s.Stop(context.Background())
	assert.NoError(t, err)
}

func TestSkeleton_ConfigFields_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	assert.Nil(t, s.ConfigFields())
}

func TestSkeleton_ConfigReceiver_StoresBytes(t *testing.T) {
	s := &Skeleton{}
	data := []byte(`{"key":"value"}`)
	err := s.ConfigReceiver(data)
	require.NoError(t, err)
	assert.Equal(t, data, s.RawConfig)
}

func TestSkeleton_GetConfig_Empty(t *testing.T) {
	s := &Skeleton{}
	var cfg map[string]string
	err := s.GetConfig(&cfg)
	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestSkeleton_GetConfig_Unmarshal(t *testing.T) {
	s := &Skeleton{}
	raw, _ := json.Marshal(map[string]string{"foo": "bar"})
	s.RawConfig = raw

	var cfg map[string]string
	err := s.GetConfig(&cfg)
	require.NoError(t, err)
	assert.Equal(t, "bar", cfg["foo"])
}

func TestSkeleton_GetConfig_InvalidJSON(t *testing.T) {
	s := &Skeleton{RawConfig: []byte(`not-json`)}
	var cfg map[string]string
	err := s.GetConfig(&cfg)
	assert.Error(t, err)
}

func TestSkeleton_RegisterRoutes_IsNoop(t *testing.T) {
	s := &Skeleton{}
	assert.NotPanics(t, func() {
		s.RegisterRoutes(nil, nil, nil, nil)
	})
}

func TestSkeleton_AvailableStorageTypes_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	assert.Nil(t, s.AvailableStorageTypes())
}

func TestSkeleton_CreateUserEngine_ReturnsNil(t *testing.T) {
	s := &Skeleton{}
	engine, err := s.CreateUserEngine(1)
	assert.NoError(t, err)
	assert.Nil(t, engine)
}

func TestManagerInit_LoadsPersistedPluginConfig(t *testing.T) {
	oldCallPluginAll := CallPluginAll
	t.Cleanup(func() {
		CallPluginAll = oldCallPluginAll
	})

	call, _, _, register := MakePlugin[Plugin](true)
	CallPluginAll = call

	plugin := &stubConfigPlugin{slug: "config-plugin"}
	register(plugin)

	db := openTestDB(t)
	dataDir := t.TempDir()
	store := newBoltPluginStore(db, plugin.slug, dataDir)
	require.NoError(t, store.SaveConfig([]byte(`{"foo":"bar"}`)))

	mgr := NewManager()
	err := mgr.Init(&StartupContext{
		StoreFactory: func(slug string) PluginStore {
			return newBoltPluginStore(db, slug, dataDir)
		},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"foo":"bar"}`, string(plugin.config))
	assert.Equal(t, 1, plugin.inits)

	stored, ok := GetStoreTyped[PluginStore](plugin.slug)
	assert.True(t, ok)
	assert.NotNil(t, stored)
}

// ── plugin_storage_provider ──
type testShardedProvider struct{}

func (testShardedProvider) Info() PluginInfo {
	return PluginInfo{SlugName: "sharded-test-provider", Name: "sharded-test-provider"}
}

func (testShardedProvider) AvailableStorageTypes() []StorageTypeInfo {
	return []StorageTypeInfo{{Name: "sharded", DisplayName: "sharded", Description: "test sharded"}}
}

func (testShardedProvider) CreateUserEngine(uint64) (StorageEngine, error) {
	return testShardedEngine{}, nil
}

func (testShardedProvider) OnUserEngineInit(uint64, func(string, StorageEngine)) error {
	return nil
}

func (testShardedProvider) ResolveVirtualPath(context.Context, uint64, string) (*VirtualPathInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no virtual path.
}

func (testShardedProvider) GetVirtualEntries(context.Context, uint64) ([]*EntryInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no virtual entries.
}

func (testShardedProvider) MigrateStorage(context.Context, string, string, func(uint64) (StorageEngine, error)) error {
	return nil
}

func (testShardedProvider) PreflightMigration(context.Context, string, string) error {
	return nil
}

func (testShardedProvider) TUSUploadComplete(context.Context, uint64, string, ReadSeekCloser) error {
	return nil
}

type testShardedEngine struct{}

func (testShardedEngine) Name() string { return "sharded" }

func (testShardedEngine) Read(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrNotFound
}

func (testShardedEngine) Stat(context.Context, string) (*FileStat, error) {
	return nil, ErrNotFound
}

func (testShardedEngine) Write(context.Context, string, io.Reader) error { return nil }
func (testShardedEngine) Mkdir(context.Context, string) error            { return nil }
func (testShardedEngine) Copy(context.Context, string, string) error     { return nil }
func (testShardedEngine) Move(context.Context, string, string) error     { return nil }
func (testShardedEngine) Delete(context.Context, string) error           { return nil }

func (testShardedEngine) List(context.Context, string) ([]EntryInfo, error) {
	return nil, nil //nolint:nilnil // storage test stub intentionally returns no entries.
}

func TestStorageSwitchToSharded(t *testing.T) {
	RegisterStorageProvider(testShardedProvider{})
	_ = StatusManager.Enable("sharded-test-provider", true)

	db := openTestDB(t)

	userStore := &boltUserStore{db: db}
	settingsStore := &boltSettingsStore{db: db}
	fileStore := &boltFileStore{db: db}
	settingsSvc := newSettingsService(settingsStore)

	dataDir, err := os.MkdirTemp("", "abyss-storage-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dataDir)

	storageSvc := newStorageService(fileStore, userStore, settingsSvc, dataDir)

	user := &User{Email: "test@example.com", Username: "sharded_test_user", UUID: "sharded-test-uuid"}
	err = userStore.Create(context.Background(), user)
	require.NoError(t, err)
	require.NotZero(t, user.ID)

	engine, err := storageSvc.GetEngine(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "path", engine.Name())

	settings, err := settingsSvc.Get(context.Background())
	require.NoError(t, err)
	settings.StorageType = "sharded"
	err = settingsSvc.Save(context.Background(), settings)
	require.NoError(t, err)

	storageSvc.engines = make(map[uint64]StorageEngine)

	engine, err = storageSvc.GetEngine(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "sharded", engine.Name())
}
