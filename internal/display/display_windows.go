//go:build windows
// +build windows

package display

import (
	"github.com/reeflective/readline/internal/core"
)

// WatchResize schedules a refresh on the Readline goroutine when the Windows
// input reader observes a console resize event.
func WatchResize(eng *Engine) chan<- bool {
	resizeChannel := core.GetTerminalResize(eng.keys)
	done := make(chan bool, 1)

	go func() {
		for {
			select {
			case <-resizeChannel:
				// Only the editor goroutine may regenerate completions or draw.
				eng.completer.RequestRegen()
				eng.keys.RequestRefresh()
			case <-done:
				return
			}
		}
	}()

	return done
}
