package api

import (
	"testing"
)

func TestIsNewer(t *testing.T) {
	s := &Server{}

	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"v0.9.9", "v1.0.0", true},
		{"v1.0.0", "v0.9.9", false},
		{"v1.0.0", "v1.0.0", false},
		{"v0.0.0-dev", "v1.0.0", false},
		{"v0.0.0-dev", "v0.0.0", false},
		{"v0.0.0-latest", "v1.0.0", false},
		{"v0.0.0-latest", "v0.0.0", false},
		{"v1.0.0-1-gabcdef", "v1.0.0", false},
		{"v1.0.0-1-gabcdef", "v1.0.1", true},
		{"v0.9.9-beta", "v0.9.10", true},
		{"v0.9.10", "v0.9.10-beta", false},
	}

	for _, tt := range tests {
		got := s.isNewer(tt.current, tt.latest)
		if got != tt.want {
			t.Errorf("isNewer(%s, %s) = %v; want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}
