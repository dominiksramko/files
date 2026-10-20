package sessions

import (
	"errors"
	"testing"
	"time"

	"files/internal/testutil"
)

// newTestStore returns a store on a fresh database whose clock is *now, plus
// a user to log in as.
func newTestStore(t *testing.T, now *time.Time) (*Store, int64) {
	t.Helper()
	db := testutil.OpenDB(t)
	store := NewStore(db)
	store.now = func() time.Time { return *now }
	return store, testutil.CreateUser(t, db, "ann", false, "/srv/users/ann")
}

func TestCreateAndLookup(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store, annID := newTestStore(t, &now)

	token, expires, err := store.Create(annID)
	if err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(now.Add(IdleTimeout)) {
		t.Errorf("expires = %v, want %v", expires, now.Add(IdleTimeout))
	}
	session, err := store.Lookup(token)
	if err != nil {
		t.Fatal(err)
	}
	want := User{ID: annID, Username: "ann", IsAdmin: false, RootDir: "/srv/users/ann"}
	if session.User != want {
		t.Errorf("user = %+v, want %+v", session.User, want)
	}
}

func TestOnlyTheTokenHashIsStored(t *testing.T) {
	now := time.Now()
	store, annID := newTestStore(t, &now)
	token, _, _ := store.Create(annID)
	var stored string
	store.db.QueryRow(`SELECT token_hash FROM sessions`).Scan(&stored)
	if stored == token || stored != hashToken(token) {
		t.Errorf("stored %q for token %q", stored, token)
	}
}

func TestLookupSeesCurrentUserDetails(t *testing.T) {
	now := time.Now()
	store, annID := newTestStore(t, &now)
	token, _, _ := store.Create(annID)

	store.db.Exec(`UPDATE users SET is_admin = 1, root_dir = '/srv/elsewhere' WHERE id = ?`, annID)
	session, _ := store.Lookup(token)
	if !session.User.IsAdmin || session.User.RootDir != "/srv/elsewhere" {
		t.Errorf("user = %+v, want the updated admin flag and root folder", session.User)
	}

	store.db.Exec(`DELETE FROM users WHERE id = ?`, annID)
	if _, err := store.Lookup(token); !errors.Is(err, ErrNoSession) {
		t.Errorf("deleted user: err = %v, want ErrNoSession", err)
	}
}

func TestUnknownTokens(t *testing.T) {
	now := time.Now()
	store, _ := newTestStore(t, &now)
	for _, token := range []string{"", "not-a-session"} {
		if _, err := store.Lookup(token); !errors.Is(err, ErrNoSession) {
			t.Errorf("Lookup(%q): err = %v, want ErrNoSession", token, err)
		}
	}
}

func TestIdleSessionEnds(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store, annID := newTestStore(t, &now)
	token, _, _ := store.Create(annID)

	now = now.Add(IdleTimeout - time.Minute)
	if _, err := store.Lookup(token); err != nil {
		t.Fatalf("just before the idle limit: %v", err)
	}
	now = now.Add(IdleTimeout) // unused for a full IdleTimeout since
	if _, err := store.Lookup(token); !errors.Is(err, ErrNoSession) {
		t.Errorf("after the idle limit: err = %v, want ErrNoSession", err)
	}
	var rows int
	store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&rows)
	if rows != 0 {
		t.Errorf("%d session rows left, want the expired one removed", rows)
	}
}

func TestUseKeepsSessionAliveUpToMaxLifetime(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	now := start
	store, annID := newTestStore(t, &now)
	token, _, _ := store.Create(annID)

	for now.Before(start.Add(MaxLifetime - 6*24*time.Hour)) {
		now = now.Add(6 * 24 * time.Hour) // used every 6 days
		session, err := store.Lookup(token)
		if err != nil {
			t.Fatalf("used on %v: %v", now, err)
		}
		if !session.Extended {
			t.Errorf("used on %v: not extended", now)
		}
	}
	now = start.Add(MaxLifetime - time.Hour)
	session, _ := store.Lookup(token)
	if !session.Expires.Equal(start.Add(MaxLifetime)) {
		t.Errorf("near the end: expires %v, want the hard limit %v", session.Expires, start.Add(MaxLifetime))
	}
	now = start.Add(MaxLifetime)
	if _, err := store.Lookup(token); !errors.Is(err, ErrNoSession) {
		t.Errorf("at the hard limit: err = %v, want ErrNoSession", err)
	}
}

func TestLastUseRecordedHourly(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store, annID := newTestStore(t, &now)
	token, _, _ := store.Create(annID)

	now = now.Add(30 * time.Minute)
	session, _ := store.Lookup(token)
	if session.Extended {
		t.Error("extended after 30 minutes; want at most hourly writes")
	}
	now = now.Add(time.Hour)
	session, _ = store.Lookup(token)
	if !session.Extended || !session.Expires.Equal(now.Add(IdleTimeout)) {
		t.Errorf("after 90 minutes: extended %v, expires %v; want %v", session.Extended, session.Expires, now.Add(IdleTimeout))
	}
}

func TestDelete(t *testing.T) {
	now := time.Now()
	store, annID := newTestStore(t, &now)
	token, _, _ := store.Create(annID)
	other, _, _ := store.Create(annID)
	if err := store.Delete(token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Lookup(token); !errors.Is(err, ErrNoSession) {
		t.Errorf("deleted session still valid")
	}
	if _, err := store.Lookup(other); err != nil {
		t.Errorf("another session ended too: %v", err)
	}
}

func TestDeleteForUser(t *testing.T) {
	now := time.Now()
	store, annID := newTestStore(t, &now)
	bobID := testutil.CreateUser(t, store.db, "bob", false, "")
	current, _, _ := store.Create(annID)
	otherDevice, _, _ := store.Create(annID)
	bobs, _, _ := store.Create(bobID)

	store.DeleteForUser(annID, current)
	if _, err := store.Lookup(current); err != nil {
		t.Errorf("kept session ended: %v", err)
	}
	if _, err := store.Lookup(otherDevice); !errors.Is(err, ErrNoSession) {
		t.Error("ann's other session still valid")
	}

	store.DeleteForUser(annID, "")
	if _, err := store.Lookup(current); !errors.Is(err, ErrNoSession) {
		t.Error("ann's last session still valid")
	}
	if _, err := store.Lookup(bobs); err != nil {
		t.Errorf("bob's session ended too: %v", err)
	}
}

func TestCreateClearsExpiredSessions(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store, annID := newTestStore(t, &now)
	store.Create(annID)
	now = now.Add(IdleTimeout + time.Minute)
	store.Create(annID)
	var rows int
	store.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&rows)
	if rows != 1 {
		t.Errorf("%d session rows, want only the new one", rows)
	}
}
