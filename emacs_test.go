package readline

import (
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/reeflective/readline/internal/core"
	"github.com/reeflective/readline/internal/history"
	"github.com/reeflective/readline/internal/macro"
)

// feedPaste builds a minimal shell, feeds a bracketed-paste payload terminated
// by the end sequence, runs the paste handler, and returns the resulting line.
func feedPaste(t *testing.T, payload string) string {
	t.Helper()

	return feedPasteWithTransformer(t, payload, nil)
}

func feedPasteWithTransformer(t *testing.T, payload string, transformer func(string) string) string {
	t.Helper()

	line := new(core.Line)
	rl := &Shell{
		Keys:   new(core.Keys),
		line:   line,
		cursor: core.NewCursor(line),
	}
	rl.PasteTransformer = transformer

	// The handler consumes keys until it sees the paste-end sequence, so the
	// terminator must be fed too — otherwise it would block waiting for input.
	rl.Keys.Feed(false, []rune(payload+"\x1b[201~")...)
	rl.bracketedPasteBegin()

	return string(*line)
}

// TestBracketedPasteNormalizesCarriageReturns guards that pasted line breaks,
// which terminals deliver as \r or \r\n, are stored as \n. Raw carriage
// returns corrupt the line buffer and its multiline rendering.
func TestBracketedPasteNormalizesCarriageReturns(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"crlf", "a\r\nb", "a\nb"},
		{"bare cr", "a\rb", "a\nb"},
		{"mixed", "one\r\ntwo\rthree", "one\ntwo\nthree"},
		{"no newline", "plain", "plain"},
		{"already lf", "a\nb", "a\nb"},
		{"unicode macro input", "hello 世界 🙂", "hello 世界 🙂"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := feedPaste(t, c.payload); got != c.want {
				t.Fatalf("paste %q => %q, want %q", c.payload, got, c.want)
			}
		})
	}
}

func TestBracketedPastePreservesFollowingInput(t *testing.T) {
	line := new(core.Line)
	rl := &Shell{Keys: new(core.Keys), line: line, cursor: core.NewCursor(line)}
	rl.Keys.PrependBuffer([]byte("hello 世界\x1b[201~\u0301next"))
	rl.bracketedPasteBegin()
	if got := string(*line); got != "hello 世界" {
		t.Fatalf("paste = %q, want hello 世界", got)
	}
	if got := string(rl.Keys.DrainBuffer()); got != "\u0301next" {
		t.Fatalf("following input = %q, want combining mark and next", got)
	}
}

func TestBracketedPasteAcrossInputReads(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		wantEOF           bool
	}{
		{"split UTF-8 and terminator", "世界\r\nhello\x1b[201~", "世界\nhello", false},
		{"unterminated paste", "partial\r\ntext", "partial\ntext", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := core.Stdin
			core.Stdin = io.NopCloser(iotest.OneByteReader(strings.NewReader(tc.input)))
			t.Cleanup(func() { core.Stdin = original })
			line := new(core.Line)
			rl := &Shell{Keys: new(core.Keys), line: line, cursor: core.NewCursor(line)}
			rl.bracketedPasteBegin()
			if got := string(*line); got != tc.want {
				t.Fatalf("paste = %q, want %q", got, tc.want)
			}
			if got := rl.Keys.IsEOF(); got != tc.wantEOF {
				t.Fatalf("EOF = %v, want %v", got, tc.wantEOF)
			}
		})
	}
}

func TestBracketedPasteMacroReplay(t *testing.T) {
	t.Setenv("INPUTRC", "/dev/null")
	rl := NewShell()
	payload := "hello 世界\r\n🙂"

	rl.Macros.StartRecord(0)
	rl.Keys.SetMatched('x')
	macro.RecordKeys(rl.Macros) // The command that starts recording is skipped.
	rl.Keys.SetMatched([]rune("\x1b[200~")...)
	rl.Keys.PrependBuffer([]byte(payload + "\x1b[201~"))
	rl.bracketedPasteBegin()
	macro.RecordKeys(rl.Macros)
	rl.Macros.StopRecord()

	if got := string(core.MacroKeys(rl.Keys)); got != "\x1b[200~"+payload+"\x1b[201~" {
		t.Fatalf("recorded paste = %q, want the complete framed payload", got)
	}

	rl.line.Set()
	rl.cursor.Set(0)
	rl.Macros.RunLastMacro()
	for _, want := range []byte("\x1b[200~") {
		got, empty := core.PopKey(rl.Keys)
		if empty || got != want {
			t.Fatalf("macro prefix byte = %q (empty %v), want %q", got, empty, want)
		}
	}
	rl.bracketedPasteBegin()
	if got, want := string(*rl.line), "hello 世界\n🙂"; got != want {
		t.Fatalf("replayed paste = %q, want %q", got, want)
	}
	if !rl.Keys.Empty() {
		t.Fatal("macro replay left unconsumed keys")
	}
}

func TestBracketedPasteTransformer(t *testing.T) {
	got := feedPasteWithTransformer(t, "a\r\nb", func(text string) string {
		if text != "a\nb" {
			t.Fatalf("transformer saw %q, want normalized text", text)
		}

		return "rewritten"
	})

	if got != "rewritten" {
		t.Fatalf("transformed paste = %q, want rewritten", got)
	}
}

func TestBracketedPasteTransformerCanDropText(t *testing.T) {
	got := feedPasteWithTransformer(t, "secret", func(string) string {
		return ""
	})

	if got != "" {
		t.Fatalf("dropped paste = %q, want empty line", got)
	}
}

// drainKeys pops everything remaining in the key buffer and returns it.
func drainKeys(rl *Shell) string {
	var b []byte

	for {
		key, empty := core.PopKey(rl.Keys)
		if empty {
			return string(b)
		}

		b = append(b, key)
	}
}

// TestSkipCsiSequence drives skip-csi-sequence over the bytes that remain after
// the dispatcher has matched the leading "\e[": it must swallow the parameter
// bytes and the single final byte, while leaving any following keystrokes
// untouched.
func TestSkipCsiSequence(t *testing.T) {
	cases := []struct {
		name     string
		feed     string // bytes left in the buffer after "\e[" was matched
		wantLeft string // what must remain unconsumed afterwards
	}{
		{"function key params and final", "15~", ""},
		{"single final byte", "Z", ""},
		{"modified arrow with params", "1;5D", ""},
		{"stops at final, keeps following keys", "15~abc", "abc"},
		{"empty buffer is a no-op", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rl := &Shell{Keys: new(core.Keys), History: &history.Sources{}}

			if c.feed != "" {
				rl.Keys.Feed(false, []rune(c.feed)...)
			}

			rl.skipCsiSequence()

			if got := drainKeys(rl); got != c.wantLeft {
				t.Fatalf("skip of %q left %q, want %q", c.feed, got, c.wantLeft)
			}
		})
	}
}
