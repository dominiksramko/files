package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"files/internal/middleware"
	"files/internal/sessions"
	"files/internal/testutil"

	"github.com/gin-gonic/gin"
)

// sessionRouter wires the login, account and user endpoints the way main.go does.
func sessionRouter(db *sql.DB) *gin.Engine {
	store := sessions.NewStore(db)
	authHandler := NewAuthHandler(db, store)
	usersHandler := NewUsersHandler(db, "/srv/users", store)
	router := gin.New()
	router.POST("/login", authHandler.Login)
	router.POST("/logout", authHandler.Logout)
	authed := router.Group("", middleware.Auth(store))
	authed.GET("/me", authHandler.Me)
	authed.PUT("/password", authHandler.ChangePassword)
	admin := authed.Group("", middleware.AdminOnly())
	admin.PUT("/users/:id", usersHandler.Update)
	admin.DELETE("/users/:id", usersHandler.Delete)
	return router
}

// call sends a request with the session token as its cookie ("" for none).
func call(t *testing.T, router http.Handler, method, target, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	request := jsonRequest(t, method, target, body)
	if token != "" {
		request.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: token})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func sessionCookieOf(response *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == middleware.SessionCookie {
			return cookie
		}
	}
	return nil
}

// login logs username in and returns the session token from the cookie.
func login(t *testing.T, router http.Handler, username string) string {
	t.Helper()
	response := call(t, router, http.MethodPost, "/login", "", map[string]string{"username": username, "password": testutil.Password})
	assertStatus(t, response, http.StatusOK)
	cookie := sessionCookieOf(response)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("no session cookie")
	}
	return cookie.Value
}

func assertLoggedIn(t *testing.T, router http.Handler, token string, want bool) {
	t.Helper()
	got := call(t, router, http.MethodGet, "/me", token, nil).Code == http.StatusOK
	if got != want {
		t.Errorf("logged in = %v, want %v", got, want)
	}
}

func TestLogin(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "/srv/users/ann")
	router := sessionRouter(db)

	response := call(t, router, http.MethodPost, "/login", "", map[string]string{"username": "ann", "password": testutil.Password})
	assertStatus(t, response, http.StatusOK)
	if strings.Contains(response.Body.String(), "token") {
		t.Errorf("response carries a token: %s", response.Body)
	}
	cookie := sessionCookieOf(response)
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v, want HttpOnly and SameSite=Strict", cookie)
	}
	assertLoggedIn(t, router, cookie.Value, true)
}

func TestLoginWrongPassword(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	for _, credentials := range []map[string]string{
		{"username": "ann", "password": "wrong"},
		{"username": "nobody", "password": testutil.Password},
	} {
		response := call(t, router, http.MethodPost, "/login", "", credentials)
		assertStatus(t, response, http.StatusUnauthorized)
		if sessionCookieOf(response) != nil {
			t.Errorf("%v: got a session cookie", credentials)
		}
	}
}

func TestLogout(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	token := login(t, router, "ann")

	response := call(t, router, http.MethodPost, "/logout", token, nil)
	assertStatus(t, response, http.StatusOK)
	if cookie := sessionCookieOf(response); cookie == nil || cookie.MaxAge >= 0 {
		t.Errorf("cookie %+v, want it cleared", cookie)
	}
	assertLoggedIn(t, router, token, false) // ended on the server, not just in the browser

	assertStatus(t, call(t, router, http.MethodPost, "/logout", "", nil), http.StatusOK)
}

func TestChangePasswordEndsOtherSessions(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	here, lostPhone := login(t, router, "ann"), login(t, router, "ann")

	response := call(t, router, http.MethodPut, "/password", here, map[string]string{"current_password": testutil.Password, "new_password": "a new one"})
	assertStatus(t, response, http.StatusOK)
	assertLoggedIn(t, router, here, true)
	assertLoggedIn(t, router, lostPhone, false)
}

func TestAdminPasswordResetEndsUsersSessions(t *testing.T) {
	db := testutil.OpenDB(t)
	adminID := testutil.CreateUser(t, db, "boss", true, "")
	annID := testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	boss, ann := login(t, router, "boss"), login(t, router, "ann")

	// A change without a new password leaves her logged in.
	assertStatus(t, call(t, router, http.MethodPut, "/users/"+strconv.FormatInt(annID, 10), boss, map[string]any{"display_name": "Ann"}), http.StatusOK)
	assertLoggedIn(t, router, ann, true)

	assertStatus(t, call(t, router, http.MethodPut, "/users/"+strconv.FormatInt(annID, 10), boss, map[string]any{"password": "reset"}), http.StatusOK)
	assertLoggedIn(t, router, ann, false)
	assertLoggedIn(t, router, boss, true)

	// An admin resetting their own password stays logged in here.
	assertStatus(t, call(t, router, http.MethodPut, "/users/"+strconv.FormatInt(adminID, 10), boss, map[string]any{"password": "mine"}), http.StatusOK)
	assertLoggedIn(t, router, boss, true)
}

func TestRemovedAdminRightsApplyAtOnce(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "boss", true, "")
	otherAdminID := testutil.CreateUser(t, db, "deputy", true, "")
	router := sessionRouter(db)
	boss, deputy := login(t, router, "boss"), login(t, router, "deputy")

	assertStatus(t, call(t, router, http.MethodPut, "/users/"+strconv.FormatInt(otherAdminID, 10), boss, map[string]any{"is_admin": false}), http.StatusOK)
	response := call(t, router, http.MethodDelete, "/users/1", deputy, nil)
	assertStatus(t, response, http.StatusForbidden)
}

func TestDeleteUserEndsSessions(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "boss", true, "")
	annID := testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	boss, ann := login(t, router, "boss"), login(t, router, "ann")

	assertStatus(t, call(t, router, http.MethodDelete, "/users/"+strconv.FormatInt(annID, 10), boss, nil), http.StatusOK)
	assertLoggedIn(t, router, ann, false)
	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, annID).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d session rows left for the deleted user", rows)
	}
}

func TestHLSPlaylistHasNoToken(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("needs ffmpeg and ffprobe (the dev container has them)")
	}
	annRoot, _ := newUserFolders(t)
	video := filepath.Join(annRoot, "clip.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=12:size=64x36:rate=5", "-pix_fmt", "yuv420p", video).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	request := httptest.NewRequest(http.MethodGet, "/?path=clip.mp4&token=leaked", nil)
	response := serve(NewHLSHandler(t.TempDir()).Playlist, annRoot, request)
	assertStatus(t, response, http.StatusOK)
	playlist := response.Body.String()
	if !strings.Contains(playlist, "segment?path=clip.mp4&seq=1\n") || strings.Contains(playlist, "token") {
		t.Errorf("playlist:\n%s", playlist)
	}
}
