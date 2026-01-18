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

func TestComputeCoordinatesCachesCursorQuery(t *testing.T) {
	eng := newTestEngine(t)

	calls := 0
	eng.cursorPos = func() (int, int) {
		calls++
		return 3, 5
	}

	eng.computeCoordinates(false)
	if calls != 1 {
		t.Fatalf("expected cursorPos to be queried once, got %d", calls)
	}

	calls = 0
	eng.computeCoordinates(false)
	if calls != 0 {
		t.Fatalf("expected cached coordinates to be reused, got %d extra queries", calls)
	}
}

func TestComputeCoordinatesTracksPromptWidthWithoutQuery(t *testing.T) {
	eng := newTestEngine(t)

	eng.cursorPos = func() (int, int) { return 2, 4 }
	eng.computeCoordinates(false)

	// Change the prompt width; the cached coordinates should update without re-querying.
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

	eng.invalidateStart()
	eng.computeCoordinates(false)
	if calls != 2 {
		t.Fatalf("expected cursorPos to be queried after invalidation, got %d", calls)
	}
}
