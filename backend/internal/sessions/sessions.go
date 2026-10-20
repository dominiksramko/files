// Package sessions keeps logins on the server. The browser holds only a random
// token (in an HttpOnly cookie); every request looks the session up together
// with the user's current details, so logging out, changing a password or
// deleting a user ends sessions immediately, and changes to a user (admin
// rights, root folder) apply at once.
package sessions

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

const (
	// IdleTimeout ends a session that hasn't been used for this long.
	IdleTimeout = 7 * 24 * time.Hour
	// MaxLifetime ends a session this long after login, however much it's used.
	MaxLifetime = 30 * 24 * time.Hour
	// touchInterval: last use is recorded at most this often, not on every request.
	touchInterval = time.Hour
)

// User is the logged-in user, as currently in the database.
type User struct {
	ID       int64
	Username string
	IsAdmin  bool
	RootDir  string
}

// Session is a valid session found for a token.
type Session struct {
	User User
	// Expires is when the session ends if it isn't used again: the cookie's expiry.
	Expires time.Time
	// Extended reports that this use pushed Expires back, so the cookie should be
	// sent again with the new expiry.
	Extended bool
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// Create starts a session for the user and returns its token (for the cookie)
// and when it expires if unused. Expired sessions are cleared out on the way.
func (s *Store) Create(userID int64) (token string, expires time.Time, err error) {
	if err := s.deleteExpired(); err != nil {
		return "", time.Time{}, err
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", time.Time{}, err
	}
	token = hex.EncodeToString(random)

	now := s.now()
	_, err = s.db.Exec(
		`INSERT INTO sessions (token_hash, user_id, created_at, last_used_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		hashToken(token), userID, now.Unix(), now.Unix(), now.Add(MaxLifetime).Unix(),
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiry(now, now.Add(MaxLifetime)), nil
}

// ErrNoSession: the token matches no live session (never existed, ended,
// expired, or its user was deleted).
var ErrNoSession = errors.New("no session")

// Lookup returns the session for token with its user's current details, and
// records the use.
func (s *Store) Lookup(token string) (*Session, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	var session Session
	var lastUsed, hardExpiry int64
	err := s.db.QueryRow(`
		SELECT u.id, u.username, u.is_admin, u.root_dir, s.last_used_at, s.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?`, hashToken(token),
	).Scan(&session.User.ID, &session.User.Username, &session.User.IsAdmin, &session.User.RootDir, &lastUsed, &hardExpiry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}

	now := s.now()
	lastUse, hardEnd := time.Unix(lastUsed, 0), time.Unix(hardExpiry, 0)
	if !now.Before(expiry(lastUse, hardEnd)) {
		s.Delete(token)
		return nil, ErrNoSession
	}
	if now.Sub(lastUse) >= touchInterval {
		if _, err := s.db.Exec(`UPDATE sessions SET last_used_at = ? WHERE token_hash = ?`, now.Unix(), hashToken(token)); err != nil {
			return nil, err
		}
		lastUse = now
		session.Extended = true
	}
	session.Expires = expiry(lastUse, hardEnd)
	return &session, nil
}

// Delete ends the session for token.
func (s *Store) Delete(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// DeleteForUser ends all of the user's sessions except the one for keepToken
// ("" ends them all).
func (s *Store) DeleteForUser(userID int64, keepToken string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, hashToken(keepToken))
	return err
}

func (s *Store) deleteExpired() error {
	now := s.now()
	_, err := s.db.Exec(
		`DELETE FROM sessions WHERE expires_at <= ? OR last_used_at <= ?`,
		now.Unix(), now.Add(-IdleTimeout).Unix(),
	)
	return err
}

// expiry is when a session last used at lastUse ends: after IdleTimeout
// without use, and never after hardEnd.
func expiry(lastUse, hardEnd time.Time) time.Time {
	idleEnd := lastUse.Add(IdleTimeout)
	if hardEnd.Before(idleEnd) {
		return hardEnd
	}
	return idleEnd
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
