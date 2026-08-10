package service

import "testing"

func TestNormalizeSnsRegistryName(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"baiyu.seedao", "baiyu", true},
		{"Alice_Seed", "alice_seed", true},
		{"ab", "", false},
		{"bad-name", "", false},
	}

	for _, tt := range tests {
		got, ok := normalizeSnsRegistryName(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("normalizeSnsRegistryName(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
