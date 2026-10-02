package display

import (
	"testing"

	"github.com/reeflective/readline/internal/completion"
	"github.com/reeflective/readline/internal/core"
	"github.com/reeflective/readline/internal/history"
	"github.com/reeflective/readline/internal/keymap"
	"github.com/reeflective/readline/internal/ui"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	t.Setenv("INPUTRC", "/dev/null")
	t.Setenv("READLINE_CURSOR_POS", "1")

	line := &core.Line{}
	cursor := core.NewCursor(line)
	selection := core.NewSelection(line, cursor)
	keys := &core.Keys{}
	iter := &core.Iterations{}
	keymaps, cfg := keymap.NewEngine(keys, iter)
	hint := &ui.Hint{}
	hist := history.NewSources(line, cursor, hint, cfg)
	comp := completion.NewEngine(hint, keymaps, cfg)
	completion.Init(comp, keys, line, cursor, selection, nil)

	prompt := ui.NewPrompt(line, cursor, keymaps, cfg)
	prompt.Primary(func() string { return "> " })

	eng := NewEngine(keys, selection, hist, prompt, hint, comp, cfg)
	Init(eng, nil)

	return eng
}

func TestComputeCoordinatesRefreshesTerminalRow(t *testing.T) {
	eng := newTestEngine(t)

	calls := 0
	eng.cursorPos = func() (int, int) {
		calls++
		return 3, 5 - calls
	}

	eng.computeCoordinates(false)
	if calls != 1 {
		t.Fatalf("expected cursorPos to be queried once, got %d", calls)
	}

	// A helper repaint may scroll the terminal between frames. Reusing the
	// previous absolute row would incorrectly reserve space at the bottom.
	eng.computeCoordinates(false)
	if calls != 2 || eng.startRows != 3 {
		t.Fatalf("expected refreshed terminal row 3, got row %d after %d queries", eng.startRows, calls)
	}
}

func TestComputeCoordinatesTracksPromptWidthWithoutQuery(t *testing.T) {
	eng := newTestEngine(t)
	_ = eng.opts.Set("cursor-position-probe", false)

	// With no probe, derive columns from the prompt and leave the absolute
	// terminal row unknown so the refresh reserves a trailing row safely.
	promptCalls := 0
	eng.cursorPos = func() (int, int) {
		promptCalls++
		return 0, 0
	}

	eng.prompt.Primary(func() string { return ">>> " })
	eng.computeCoordinates(false)

	if promptCalls != 0 {
		t.Fatalf("unexpected cursorPos query after prompt change, called %d times", promptCalls)
	}

	if got, want := eng.startCols, eng.prompt.LastUsed(); got != want {
		t.Fatalf("startCols mismatch after prompt change, got %d want %d", got, want)
	}
	if eng.startCols != 4 || eng.startRowKnown() {
		t.Fatalf("fallback coordinates = (%d, %d), want (4, unknown)", eng.startCols, eng.startRows)
	}
}

func TestComputeCoordinatesInvalidationForcesQuery(t *testing.T) {
	eng := newTestEngine(t)

	calls := 0
	eng.cursorPos = func() (int, int) {
		calls++
		return 7, 9
	}

	eng.computeCoordinates(false)
	if calls != 1 {
		t.Fatalf("expected initial cursorPos query, got %d", calls)
	}

	eng.MarkCursorDirty()
	eng.computeCoordinates(false)
	if calls != 2 {
		t.Fatalf("expected cursorPos to be queried after invalidation, got %d", calls)
	}
}

func TestComputeCoordinatesHonorsEnvironmentProbePolicy(t *testing.T) {
	for _, env := range []struct {
		name, value, program string
	}{
		{"disabled", "off", ""},
		{"iTerm", "", "iTerm.app"},
	} {
		t.Run(env.name, func(t *testing.T) {
			eng := newTestEngine(t)
			t.Setenv("READLINE_CURSOR_POS", env.value)
			t.Setenv("TERM_PROGRAM", env.program)
			eng.cursorPos = func() (int, int) {
				t.Fatal("unexpected terminal probe")
				return 0, 0
			}
			eng.computeCoordinates(false)
			if eng.startCols != 2 || eng.startRowKnown() {
				t.Fatalf("fallback coordinates = (%d, %d), want (2, unknown)", eng.startCols, eng.startRows)
			}
		})
	}
}
