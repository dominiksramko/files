package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"files/internal/sessions"

	"github.com/gin-gonic/gin"
)

// SessionCookie holds the session token; see sessions.Store.
const SessionCookie = "files_session"

// Auth lets a request through only with a live session cookie, and puts the
// user's current details in the context: userID, username, isAdmin, rootDir,
// plus sessionToken. The cookie is sent again whenever the session's expiry
// moves, so it keeps alive while used.
func Auth(store *sessions.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, _ := c.Cookie(SessionCookie)
		session, err := store.Lookup(token)
		if errors.Is(err, sessions.ErrNoSession) {
			if token != "" {
				ClearSessionCookie(c)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not logged in"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not check session"})
			return
		}
		if session.Extended {
			SetSessionCookie(c, token, session.Expires)
		}

		c.Set("userID", session.User.ID)
		c.Set("username", session.User.Username)
		c.Set("isAdmin", session.User.IsAdmin)
		c.Set("rootDir", session.User.RootDir)
		c.Set("sessionToken", token)
		c.Next()
	}
}

// SetSessionCookie sends the session cookie: HttpOnly (page scripts can't read
// it), SameSite=Strict (other sites' pages and links don't send it), and Secure
// when the request came over HTTPS, directly or through a proxy — plain-HTTP
// setups on a trusted network would otherwise be unable to log in.
func SetSessionCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   isHTTPS(c.Request),
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie tells the browser to drop the session cookie.
func ClearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(c.Request),
		SameSite: http.SameSiteStrictMode,
	})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SameOrigin refuses requests that change something (anything but GET, HEAD
// and OPTIONS) when the browser marks them as coming from another site or
// origin. The session cookie is SameSite=Strict, but a "site" spans
// subdomains, so a page on another subdomain of the same domain could still
// make the browser send it. Clients that don't send Sec-Fetch-Site (old
// browsers, scripts) are let through.
func SameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if site := c.GetHeader("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "request from another site refused"})
				return
			}
		}
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !c.GetBool("isAdmin") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}
		c.Next()
	}
}
