package config

import (
	"slices"
	"testing"
)

func TestTrustedProxies(t *testing.T) {
	tests := []struct {
		value string
		want  []string
	}{
		{"", DefaultTrustedProxies},
		{"none", nil},
		{"10.1.2.3", []string{"10.1.2.3"}},
		{" 172.18.0.0/16 , 10.1.2.3,, ", []string{"172.18.0.0/16", "10.1.2.3"}},
	}
	for _, test := range tests {
		t.Setenv("TRUSTED_PROXIES", test.value)
		if got := Load().TrustedProxies; !slices.Equal(got, test.want) {
			t.Errorf("TRUSTED_PROXIES=%q: %q, want %q", test.value, got, test.want)
		}
	}
}
