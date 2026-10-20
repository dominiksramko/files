package handlers

import "testing"

// Request paths are checked by userfs (see its tests); isWithin compares
// real locations for the "into itself" check and archive entries.
func TestIsWithin(t *testing.T) {
	tests := []struct {
		dir, abs string
		want     bool
	}{
		{"/srv/users/ann", "/srv/users/ann", true},
		{"/srv/users/ann", "/srv/users/ann/docs", true},
		{"/srv/users/ann", "/srv/users/anna", false},
		{"/srv/users/ann", "/srv/users", false},
		{"/srv/users/ann", "/srv/users/ann/..data", true},
		{"/", "/etc", true},
	}
	for _, test := range tests {
		if got := isWithin(test.dir, test.abs); got != test.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", test.dir, test.abs, got, test.want)
		}
	}
}
