//go:build windows

package core

import "golang.org/x/sys/windows"

// InitWake creates an auto-reset event used to interrupt the native console
// reader. Custom Stdin readers retain their normal blocking-read behavior.
// Like the Unix wake pipe, its lifetime belongs to the active Readline call.
func (k *Keys) InitWake() {
	k.CloseWake()
	if _, ok := Stdin.(*rawReader); !ok {
		return
	}

	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return
	}

	k.wakeMu.Lock()
	k.wakeR = int(event)
	k.wakeReady = true
	k.wakeMu.Unlock()
}

// CloseWake releases the event after the input loop has stopped reading.
// It never closes or injects synthetic input into the process's console.
func (k *Keys) CloseWake() {
	k.wakeMu.Lock()
	defer k.wakeMu.Unlock()

	if !k.wakeReady {
		return
	}
	_ = windows.CloseHandle(windows.Handle(k.wakeR))
	k.wakeR = 0
	k.wakeReady = false
}

// RequestRefresh wakes an idle native Windows console reader. Repeated
// requests coalesce in the event; no keyboard input is consumed or invented.
func (k *Keys) RequestRefresh() {
	k.wakeMu.Lock()
	defer k.wakeMu.Unlock()

	if k.wakeReady {
		_ = windows.SetEvent(windows.Handle(k.wakeR))
	}
}

func (k *Keys) waitConsoleInput() error {
	k.wakeMu.Lock()
	event, ready := windows.Handle(k.wakeR), k.wakeReady
	k.wakeMu.Unlock()
	if !ready {
		return nil
	}

	// Console input handles become signaled when there are unread records.
	// Putting stdin first gives actual input priority if both are signaled.
	which, err := windows.WaitForMultipleObjects([]windows.Handle{windows.Handle(stdin), event}, false, windows.INFINITE)
	if err != nil {
		return err
	}
	if which == windows.WAIT_OBJECT_0+1 {
		return errInputWake
	}
	return nil
}
