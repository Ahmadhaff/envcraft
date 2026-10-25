package checker

import "testing"

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		actual  string
		minimum string
		ok      bool
	}{
		{"go1.26.5", "1.26", true},
		{"go1.25.0", "1.26", false},
		{"v22.16.0", "22", true},
		{"v21.9.0", "22", false},
		{"go1.26.0", "1.26.0", true},
		{"ready", "1", false},
	}
	for _, tc := range cases {
		if got := versionAtLeast(tc.actual, tc.minimum); got != tc.ok {
			t.Fatalf("%s >= %s: got %v", tc.actual, tc.minimum, got)
		}
	}
}
