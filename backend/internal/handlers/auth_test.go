package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"files/internal/loginlimit"
	"files/internal/middleware"
	"files/internal/sessions"
	"files/internal/testutil"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
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
	admin.POST("/users", usersHandler.Create)
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

	assertStatus(t, call(t, router, http.MethodPut, "/users/"+strconv.FormatInt(annID, 10), boss, map[string]any{"password": "a reset password"}), http.StatusOK)
	assertLoggedIn(t, router, ann, false)
	assertLoggedIn(t, router, boss, true)

	// An admin resetting their own password stays logged in here.
	assertStatus(t, call(t, router, http.MethodPut, "/users/"+strconv.FormatInt(adminID, 10), boss, map[string]any{"password": "my new password"}), http.StatusOK)
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

// loginFrom attempts a login as username from the client address.
func loginFrom(t *testing.T, router http.Handler, address, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	request := jsonRequest(t, http.MethodPost, "/login", map[string]string{"username": username, "password": password})
	request.RemoteAddr = address + ":40000"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestLoginLimitsFailedAttempts(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)

	for i := 0; i < loginlimit.MaxFailures; i++ {
		assertStatus(t, loginFrom(t, router, "203.0.113.9", "ann", "wrong"), http.StatusUnauthorized)
	}
	// Refused now, even with the right password, and says how long to wait.
	response := loginFrom(t, router, "203.0.113.9", "ann", testutil.Password)
	assertStatus(t, response, http.StatusTooManyRequests)
	if retry, _ := strconv.Atoi(response.Header().Get("Retry-After")); retry < 14*60 || retry > 15*60 {
		t.Errorf("Retry-After = %q, want about 15 minutes", response.Header().Get("Retry-After"))
	}
	if !strings.Contains(response.Body.String(), "Try again in 15 minutes") {
		t.Errorf("body = %s", response.Body)
	}
	if sessionCookieOf(response) != nil {
		t.Error("a refused login got a session")
	}
}

func TestLoginLimitLeavesOthersAlone(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "")
	testutil.CreateUser(t, db, "bob", false, "")
	router := sessionRouter(db)
	for i := 0; i < loginlimit.MaxFailures; i++ {
		loginFrom(t, router, "203.0.113.9", "ann", "wrong")
	}
	// bob, from his own address, logs in as usual; ann's username is slowed
	// down to one attempt a minute (not locked) from any address.
	assertStatus(t, loginFrom(t, router, "198.51.100.7", "bob", testutil.Password), http.StatusOK)
	response := loginFrom(t, router, "198.51.100.8", "ann", testutil.Password)
	assertStatus(t, response, http.StatusTooManyRequests)
	if !strings.Contains(response.Body.String(), "in a minute") {
		t.Errorf("body = %s", response.Body)
	}
}

func TestSuccessfulLoginResetsTheCount(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	for round := 0; round < 3; round++ {
		for i := 0; i < loginlimit.MaxFailures-1; i++ {
			assertStatus(t, loginFrom(t, router, "203.0.113.9", "ann", "typo"), http.StatusUnauthorized)
		}
		assertStatus(t, loginFrom(t, router, "203.0.113.9", "ann", testutil.Password), http.StatusOK)
	}
}

func TestTooManyAttemptsMessage(t *testing.T) {
	for wait, want := range map[time.Duration]string{
		10 * time.Second: "Try again in a minute.",
		time.Minute:      "Try again in a minute.",
		61 * time.Second: "Try again in 2 minutes.",
		15 * time.Minute: "Try again in 15 minutes.",
	} {
		if got := tooManyAttemptsMessage(wait); !strings.HasSuffix(got, want) {
			t.Errorf("%v: %q, want …%q", wait, got, want)
		}
	}
}

func TestUnknownUsernameTakesAsLongAsWrongPassword(t *testing.T) {
	db := testutil.OpenDB(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("real password"), bcrypt.DefaultCost) // as real accounts are hashed
	db.Exec(`INSERT INTO users (username, password) VALUES ('ann', ?)`, string(hash))
	router := sessionRouter(db)

	attempt := 0
	median := func(username string) time.Duration {
		var durations []time.Duration
		for i := 0; i < 5; i++ {
			attempt++ // a new address each time, to stay under the attempt limit
			start := time.Now()
			assertStatus(t, loginFrom(t, router, fmt.Sprintf("198.51.100.%d", attempt), username, "wrong"), http.StatusUnauthorized)
			durations = append(durations, time.Since(start))
		}
		slices.Sort(durations)
		return durations[2]
	}
	median("warm-up")
	known, unknown := median("ann"), median("nobody")
	if unknown < known/2 || unknown > known*2 {
		t.Errorf("wrong password for an existing user: %v; for an unknown user: %v", known, unknown)
	}
}

func TestValidateNewPassword(t *testing.T) {
	for password, ok := range map[string]bool{
		"":            false,
		"1234567":     false,
		"12345678":    true,
		"ünïcödé":     false, // 7 characters, more bytes
		"ünïcödé!":    true,
		"long enough": true,
	} {
		if err := validateNewPassword(password); (err == nil) != ok {
			t.Errorf("validateNewPassword(%q) = %v", password, err)
		}
	}
}

func TestShortPasswordsRefused(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "boss", true, "")
	annID := testutil.CreateUser(t, db, "ann", false, "")
	router := sessionRouter(db)
	boss := login(t, router, "boss")
	users := strconv.FormatInt(annID, 10)

	for name, response := range map[string]*httptest.ResponseRecorder{
		"own password change": call(t, router, http.MethodPut, "/password", boss, map[string]string{"current_password": testutil.Password, "new_password": "short"}),
		"admin reset":         call(t, router, http.MethodPut, "/users/"+users, boss, map[string]any{"password": "short"}),
		"new user":            call(t, router, http.MethodPost, "/users", boss, map[string]any{"username": "cat", "password": "short"}),
	} {
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "at least 8 characters") {
			t.Errorf("%s: %d %s", name, response.Code, response.Body)
		}
	}
	// The old passwords still work: ann's was set before the rule and is 13 characters anyway.
	assertStatus(t, loginFrom(t, router, "198.51.100.1", "ann", testutil.Password), http.StatusOK)
}

func TestNewUserGetsAFolder(t *testing.T) {
	db := testutil.OpenDB(t)
	testutil.CreateUser(t, db, "boss", true, "")
	usersRoot := t.TempDir()
	store := sessions.NewStore(db)
	router := gin.New()
	router.POST("/login", NewAuthHandler(db, store).Login)
	router.POST("/users", middleware.Auth(store), middleware.AdminOnly(), NewUsersHandler(db, usersRoot, store).Create)
	boss := login(t, router, "boss")

	response := call(t, router, http.MethodPost, "/users", boss, map[string]any{"username": "cat", "password": "long enough"})
	assertStatus(t, response, http.StatusCreated)
	if info, err := os.Stat(filepath.Join(usersRoot, "cat")); err != nil || !info.IsDir() {
		t.Errorf("new user's folder not created: %v", err)
	}

	// A folder given by the admin is used as is, not created.
	given := filepath.Join(t.TempDir(), "elsewhere")
	assertStatus(t, call(t, router, http.MethodPost, "/users", boss, map[string]any{"username": "dog", "password": "long enough", "root_dir": given}), http.StatusCreated)
	assertMissing(t, given)

	// A username that can't be a folder name needs a folder given.
	assertStatus(t, call(t, router, http.MethodPost, "/users", boss, map[string]any{"username": "..", "password": "long enough"}), http.StatusBadRequest)
}
