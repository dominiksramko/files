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

func TestCacheSizeLimits(t *testing.T) {
	const gigabyte = 1 << 30
	tests := []struct {
		value string
		want  int64
	}{
		{"", 20 * gigabyte},
		{"5", 5 * gigabyte},
		{"0.5", gigabyte / 2},
		{"0", 0},
		{"lots", 20 * gigabyte},
		{"-1", 20 * gigabyte},
	}
	for _, test := range tests {
		t.Setenv("HLS_CACHE_MAX_GB", test.value)
		if got := Load().HLSCacheMaxBytes; got != test.want {
			t.Errorf("HLS_CACHE_MAX_GB=%q: %d, want %d", test.value, got, test.want)
		}
	}
	t.Setenv("CONV_CACHE_MAX_GB", "")
	if got := Load().ConvCacheMaxBytes; got != 2*gigabyte {
		t.Errorf("CONV_CACHE_MAX_GB default: %d, want %d", got, 2*gigabyte)
	}
}
