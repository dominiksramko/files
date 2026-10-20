package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"files/internal/sessions"
	"files/internal/testutil"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// protectedRouter serves GET and POST / behind Auth, answering with what the
// request's context says about the user.
func protectedRouter(store *sessions.Store) *gin.Engine {
	router := gin.New()
	whoami := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.GetInt64("userID"), "username": c.GetString("username"),
			"is_admin": c.GetBool("isAdmin"), "root_dir": c.GetString("rootDir"),
		})
	}
	router.GET("/", Auth(store), whoami)
	return router
}

func get(router http.Handler, cookie string, prepare ...func(*http.Request)) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	}
	for _, change := range prepare {
		change(request)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func sessionCookie(response *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == SessionCookie {
			return cookie
		}
	}
	return nil
}

func TestAuthRequiresSession(t *testing.T) {
	store := sessions.NewStore(testutil.OpenDB(t))
	router := protectedRouter(store)
	if response := get(router, ""); response.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: status %d, want 401", response.Code)
	}
	response := get(router, "made-up")
	if response.Code != http.StatusUnauthorized {
		t.Errorf("unknown session: status %d, want 401", response.Code)
	}
	if cookie := sessionCookie(response); cookie == nil || cookie.MaxAge >= 0 {
		t.Errorf("unknown session: cookie %+v, want it cleared", cookie)
	}
}

func TestAuthUsesCurrentUserDetails(t *testing.T) {
	db := testutil.OpenDB(t)
	store := sessions.NewStore(db)
	annID := testutil.CreateUser(t, db, "ann", true, "/srv/users/ann")
	token, _, _ := store.Create(annID)
	router := protectedRouter(store)

	response := get(router, token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"is_admin":true`) {
		t.Fatalf("status %d, body %s", response.Code, response.Body)
	}

	// Admin rights removed and folder moved: the live session follows at once.
	db.Exec(`UPDATE users SET is_admin = 0, root_dir = '/srv/elsewhere' WHERE id = ?`, annID)
	body := get(router, token).Body.String()
	if !strings.Contains(body, `"is_admin":false`) || !strings.Contains(body, `"root_dir":"/srv/elsewhere"`) {
		t.Errorf("after the change: %s", body)
	}

	db.Exec(`DELETE FROM users WHERE id = ?`, annID)
	if response := get(router, token); response.Code != http.StatusUnauthorized {
		t.Errorf("deleted user: status %d, want 401", response.Code)
	}
}

func TestAuthIgnoresTokenInURL(t *testing.T) {
	db := testutil.OpenDB(t)
	store := sessions.NewStore(db)
	token, _, _ := store.Create(testutil.CreateUser(t, db, "ann", false, ""))
	response := get(protectedRouter(store), "", func(request *http.Request) {
		request.URL.RawQuery = "token=" + token
		request.Header.Set("Authorization", "Bearer "+token)
	})
	if response.Code != http.StatusUnauthorized {
		t.Errorf("token in the URL / header: status %d, want 401", response.Code)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	for _, test := range []struct {
		name   string
		https  func(*http.Request)
		secure bool
	}{
		{"plain HTTP", func(*http.Request) {}, false},
		{"HTTPS via a proxy", func(request *http.Request) { request.Header.Set("X-Forwarded-Proto", "https") }, true},
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		test.https(ctx.Request)
		SetSessionCookie(ctx, "token", time.Now().Add(time.Hour))
		cookie := sessionCookie(recorder)
		if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Secure != test.secure {
			t.Errorf("%s: cookie %+v", test.name, cookie)
		}
		if cookie != nil && (cookie.MaxAge < 3500 || cookie.MaxAge > 3600) {
			t.Errorf("%s: MaxAge %d, want about an hour", test.name, cookie.MaxAge)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	router := gin.New()
	router.Use(SameOrigin())
	router.Any("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	tests := []struct {
		method, fetchSite string
		want              int
	}{
		{http.MethodPost, "cross-site", http.StatusForbidden},
		{http.MethodPost, "same-site", http.StatusForbidden}, // another subdomain
		{http.MethodDelete, "cross-site", http.StatusForbidden},
		{http.MethodPost, "same-origin", http.StatusOK},
		{http.MethodPost, "none", http.StatusOK},
		{http.MethodPost, "", http.StatusOK}, // old browsers, scripts
		{http.MethodGet, "cross-site", http.StatusOK},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, "/", nil)
		if test.fetchSite != "" {
			request.Header.Set("Sec-Fetch-Site", test.fetchSite)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != test.want {
			t.Errorf("%s with Sec-Fetch-Site %q: status %d, want %d", test.method, test.fetchSite, recorder.Code, test.want)
		}
	}
}

func TestAuthRenewsCookieWhileUsed(t *testing.T) {
	db := testutil.OpenDB(t)
	store := sessions.NewStore(db)
	token, _, _ := store.Create(testutil.CreateUser(t, db, "ann", false, ""))
	router := protectedRouter(store)

	if cookie := sessionCookie(get(router, token)); cookie != nil {
		t.Errorf("used again at once: cookie %+v sent, want none", cookie)
	}
	db.Exec(`UPDATE sessions SET last_used_at = ?`, time.Now().Add(-2*time.Hour).Unix())
	cookie := sessionCookie(get(router, token))
	if cookie == nil || cookie.Value != token || cookie.MaxAge < int(sessions.IdleTimeout.Seconds())-60 {
		t.Errorf("used after two hours: cookie %+v, want it renewed for another %v", cookie, sessions.IdleTimeout)
	}
}
