//go:build unix

package display_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestAsyncOutputPreservesInputAndCompletions(t *testing.T) {
	for _, noProbe := range []bool{false, true} {
		t.Run(fmt.Sprintf("probe=%v", !noProbe), func(t *testing.T) {
			c := startConsole(t, consoleConfig{
				prompt: "PROMPT> ", cols: 80, rows: 50,
				noProbe: noProbe, asyncOutput: true,
				autocomplete: true, hintProvider: true,
			})
			c.waitForScreen("PROMPT>")
			c.send("al")
			c.waitForScreen("LOG-DONE")
			c.waitForScreen("PROMPT> al")
			c.waitForScreen("alpha")
			for i := range 12 {
				message := fmt.Sprintf("LOG-%02d", i)
				if got := strings.Count(c.rawOutput(), message); got != 1 {
					t.Fatalf("printed %s %d times, want once", message, got)
				}
			}
			c.send("phabet\r")
			c.waitForScreen("[LINE:alphabet]")
		})
	}
}

func TestOutputFromCallbacksAndReadlineExit(t *testing.T) {
	c := startConsole(t, consoleConfig{
		prompt: "PROMPT> ", cols: 80, rows: 24, outputCallbacks: true,
	})
	c.waitForScreen("PROMPT-OUTPUT")
	c.waitForScreen("PROMPT>")
	c.send("hello")
	c.waitForScreen("HIGHLIGHTER-OUTPUT")
	c.waitForScreen("PROMPT> hello")
	c.send("\r")
	c.waitForScreen("[LINE:hello]")
	if got := strings.Count(c.rawOutput(), "ACCEPT-OUTPUT"); got != 1 {
		t.Fatalf("Readline exit printed callback output %d times, want once", got)
	}
}

func TestInitialCallbackOutputAfterWrappedReadline(t *testing.T) {
	c := startConsole(t, consoleConfig{
		prompt: "PROMPT> ", cols: 20, rows: 30, repeatOutput: true,
	})
	c.waitForScreen("PROMPT>")
	c.send(strings.Repeat("x", 40) + "\r")
	c.waitForScreen("NEXT-OUTPUT")
	screen := c.waitForScreen("NEXT>")
	if !strings.Contains(screen, "FIRST-DONE") {
		t.Fatalf("second prompt output overwrote the previous result:\n%s", screen)
	}
	c.send("next\r")
	c.waitForScreen("[SECOND:next]")
}
