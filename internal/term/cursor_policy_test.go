package term

import "testing"

func TestCursorProbePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, value, program string
		want                 bool
	}{
		{"default", "", "", true},
		{"iTerm default", "", "iTerm.app", false},
		{"explicit opt in", " ON ", "iTerm.app", true},
		{"explicit opt out", "false", "", false},
		{"invalid value", "invalid", "iTerm.app", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("READLINE_CURSOR_POS", tc.value)
			t.Setenv("TERM_PROGRAM", tc.program)
			if got := ShouldQueryCursorPos(); got != tc.want {
				t.Fatalf("ShouldQueryCursorPos() = %v, want %v", got, tc.want)
			}
		})
	}
}
