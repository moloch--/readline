//go:build windows

package core

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func windowsWakeKeys(t *testing.T) (*Keys, windows.Handle) {
	t.Helper()
	oldStdin, oldHandle := Stdin, stdin
	Stdin = newRawReader()
	t.Cleanup(func() { Stdin, stdin = oldStdin, oldHandle })

	// An event stands in for the waitable console handle, so these tests also
	// work on Windows CI without an interactive console attached.
	input, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	stdin = uintptr(input)
	t.Cleanup(func() { _ = windows.CloseHandle(input) })

	keys := &Keys{}
	keys.InitWake()
	if !keys.wakeReady {
		t.Fatal("wake event was not created")
	}
	t.Cleanup(keys.CloseWake)
	return keys, input
}

func TestWindowsRefreshWakesIdleReader(t *testing.T) {
	keys, _ := windowsWakeKeys(t)
	result := make(chan error, 1)
	go func() { result <- keys.waitConsoleInput() }()
	keys.RequestRefresh()

	select {
	case err := <-result:
		if !errors.Is(err, errInputWake) {
			t.Fatalf("wake = %v, want input wake", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle console reader did not wake")
	}

	// The auto-reset event must not leave the next read spinning.
	state, err := windows.WaitForSingleObject(windows.Handle(keys.wakeR), 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("consumed wake remains signaled: state=%d err=%v", state, err)
	}
}

func TestWindowsInputTakesPriorityOverRefresh(t *testing.T) {
	keys, input := windowsWakeKeys(t)
	if err := windows.SetEvent(input); err != nil {
		t.Fatal(err)
	}
	keys.RequestRefresh()
	keys.RequestRefresh() // multiple requests coalesce
	if err := keys.waitConsoleInput(); err != nil {
		t.Fatalf("readable input should precede the wake: %v", err)
	}
	if err := windows.ResetEvent(input); err != nil {
		t.Fatal(err)
	}
	if err := keys.waitConsoleInput(); !errors.Is(err, errInputWake) {
		t.Fatalf("pending wake = %v", err)
	}
	state, err := windows.WaitForSingleObject(windows.Handle(keys.wakeR), 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("duplicate requests did not coalesce: state=%d err=%v", state, err)
	}
}

func TestWindowsWakeLifecycle(t *testing.T) {
	keys, _ := windowsWakeKeys(t)
	var requests sync.WaitGroup
	requests.Add(1)
	go func() {
		defer requests.Done()
		for range 100 {
			keys.RequestRefresh()
		}
	}()
	keys.CloseWake()
	requests.Wait()
	keys.CloseWake()
	keys.RequestRefresh()
	if keys.wakeReady || keys.wakeR != 0 {
		t.Fatal("closed wake still marked ready")
	}
	keys.InitWake()
	keys.RequestRefresh()
	if err := keys.waitConsoleInput(); !errors.Is(err, errInputWake) {
		t.Fatalf("reinitialized wake = %v", err)
	}
}

func TestWindowsCustomReaderFallback(t *testing.T) {
	oldStdin := Stdin
	Stdin = io.NopCloser(strings.NewReader("x"))
	defer func() { Stdin = oldStdin }()
	keys := &Keys{}
	keys.InitWake()
	defer keys.CloseWake()
	keys.RequestRefresh()
	if keys.wakeReady {
		t.Fatal("custom reader must not create an unusable console wake")
	}
	got, err := keys.readInputFiltered()
	if err != nil || string(got) != "x" {
		t.Fatalf("custom input = %q, %v", got, err)
	}
}

func TestWindowsRawReaderRechecksWakeAfterIgnoredEvent(t *testing.T) {
	reads := 0
	readRecord := func(record *_INPUT_RECORD) error {
		reads++
		record.EventType = EVENT_KEY // zero bKeyDown: ignored key-release
		return nil
	}
	waits := 0
	_, err := newRawReader().readRecords(make([]byte, keyScanBufSize), func() error {
		waits++
		if waits > 1 {
			return errInputWake
		}
		return nil
	}, readRecord)
	if !errors.Is(err, errInputWake) || reads != 1 || waits != 2 {
		t.Fatalf("ignored-event wake: reads=%d waits=%d err=%v", reads, waits, err)
	}
}

func TestWindowsRawReaderPreservesInputAndReadErrors(t *testing.T) {
	readRecord := func(record *_INPUT_RECORD) error {
		record.EventType = EVENT_KEY
		key := (*_KEY_EVENT_RECORD)(unsafe.Pointer(&record.Event[0]))
		key.bKeyDown, key.unicodeChar = 1, 'x'
		return nil
	}
	buf := make([]byte, keyScanBufSize)
	n, err := newRawReader().readRecords(buf, func() error { return nil }, readRecord)
	if err != nil || string(buf[:n]) != "x" {
		t.Fatalf("native input = %q, %v", buf[:n], err)
	}
	_, err = newRawReader().readRecords(buf, func() error { return nil }, func(*_INPUT_RECORD) error {
		return windows.ERROR_INVALID_HANDLE
	})
	if !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		t.Fatalf("native read failure = %v", err)
	}
}
