//go:build windows

package main

// A test of the console reader against the real console.
//
// It writes a key into the console of whoever is running it, so it only
// runs when it is asked for:
//
//	GITPICKER_CONSOLE_TEST=1 go test -run TestTheConsoleReader
//
// The console is shared with the shell, so the test empties the input
// buffer when it is done.

import (
	"bytes"
	"encoding/binary"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

// writeConsoleInput puts records into the console input buffer.
//
// The console has a wide and a narrow version of this call, and the wide
// one is the one that is here.
var writeConsoleInput = syscall.NewLazyDLL("kernel32.dll").NewProc("WriteConsoleInputW")

// openConsoleInput opens the console input device.
//
// The console is opened by its name and not through the standard input,
// because the input of a test binary is not a console.
func openConsoleInput(t *testing.T) windows.Handle {
	t.Helper()

	name, err := windows.UTF16PtrFromString("CONIN$")
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		t.Skipf("there is no console here: %v", err)
	}
	return handle
}

// pushKey writes a key press into the console input buffer.
//
// This is what the keyboard does, so the reader cannot tell the difference
// between a key that was pushed in and one that was typed.
func pushKey(t *testing.T, handle windows.Handle, code coninput.VirtualKeyCode, char rune, state coninput.ControlKeyState) {
	t.Helper()

	// The record is the layout the console expects: the kind of the
	// event, and then the key event itself.
	record := make([]byte, 20)
	binary.LittleEndian.PutUint16(record[0:2], 1)  // a key event
	binary.LittleEndian.PutUint32(record[4:8], 1)  // the key is down
	binary.LittleEndian.PutUint16(record[8:10], 1) // one press
	binary.LittleEndian.PutUint16(record[10:12], uint16(code))
	binary.LittleEndian.PutUint16(record[14:16], uint16(char))
	binary.LittleEndian.PutUint32(record[16:20], uint32(state))

	var written uint32
	result, _, err := writeConsoleInput.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&record[0])),
		1,
		uintptr(unsafe.Pointer(&written)),
	)
	if result == 0 {
		t.Fatalf("the key could not be pushed into the console: %v", err)
	}
	if written != 1 {
		t.Fatalf("the console took %d records, want 1", written)
	}
}

// recordingModel is the picker with the keys it is given written down.
type recordingModel struct {
	model
	keys chan tea.KeyMsg
}

// Init starts the picker.
func (m recordingModel) Init() tea.Cmd {
	return m.model.Init()
}

// Update records the keys and carries on with the picker.
func (m recordingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		select {
		case m.keys <- key:
		default:
		}
	}
	next, cmd := m.model.Update(msg)
	inner, ok := next.(model)
	if !ok {
		return next, cmd
	}
	return recordingModel{model: inner, keys: m.keys}, cmd
}

// View draws the picker.
func (m recordingModel) View() string {
	return m.model.View()
}

func TestTheConsoleReaderReadsAKeyThatIsPushedIn(t *testing.T) {
	if os.Getenv("GITPICKER_CONSOLE_TEST") == "" {
		t.Skip("the console test only runs when it is asked for")
	}

	// The keyboard is the one the picker uses, so the test covers the
	// whole way a key travels.
	keyboard := openKeyboard()
	if len(keyboard.options) == 0 {
		t.Skip("there is no console to read here")
	}

	// The key is pushed in through a handle of its own. The console
	// input is shared, so the reader sees it.
	handle := openConsoleInput(t)
	defer coninput.FlushConsoleInputBuffer(handle)

	m := testModel("master")
	m.phase = phaseMessage
	keys := make(chan tea.KeyMsg, 8)

	var out bytes.Buffer
	options := append(
		[]tea.ProgramOption{tea.WithAltScreen(), tea.WithOutput(&out)},
		keyboard.options...,
	)
	program := tea.NewProgram(recordingModel{model: m, keys: keys}, options...)

	stop := keyboard.begin(program)
	defer stop()

	failures := make(chan error, 1)
	go func() {
		_, err := program.Run()
		failures <- err
	}()

	// The program needs a moment to start reading.
	time.Sleep(200 * time.Millisecond)

	// Ctrl+Enter, pushed into the console as the keyboard would.
	pushKey(t, handle, coninput.VK_RETURN, '\n', coninput.LEFT_CTRL_PRESSED)

	select {
	case key := <-keys:
		if key.Type != tea.KeyCtrlJ {
			t.Fatalf("the key: got type %d (%q), want %d", key.Type, key.String(), tea.KeyCtrlJ)
		}
		if key.String() != "ctrl+j" {
			t.Fatalf("the key should be the ctrl+j key, got %q", key.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the key never came through")
	}

	// And a plain enter is still plain enter.
	pushKey(t, handle, coninput.VK_RETURN, '\r', 0)

	select {
	case key := <-keys:
		if key.Type != tea.KeyEnter {
			t.Fatalf("the key: got type %d (%q), want %d", key.Type, key.String(), tea.KeyEnter)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the key never came through")
	}

	// The picker is still alive, so it is stopped the way it is stopped
	// when the user leaves.
	program.Quit()
	select {
	case err := <-failures:
		if err != nil {
			t.Fatalf("the picker ended badly: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the picker did not end")
	}
}
