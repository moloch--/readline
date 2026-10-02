package display

import (
	"strings"
	"testing"

	"github.com/reeflective/readline/internal/color"
)

func TestEmptyCommentTokenDisablesHighlight(t *testing.T) {
	eng := newTestEngine(t)
	line := []rune("echo # comment")
	_ = eng.opts.Set("comment-begin", "#")
	if got := eng.highlightLine(line, *eng.selection); !strings.Contains(got, "244") {
		t.Fatalf("expected comment highlighting, got %q", got)
	}

	_ = eng.opts.Set("comment-begin", "")
	if got, want := eng.highlightLine(line, *eng.selection), string(line)+color.Reset; got != want {
		t.Fatalf("empty comment token rendered %q, want %q", got, want)
	}
}
