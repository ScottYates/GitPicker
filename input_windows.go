//go:build windows

package main

// The console reader.
//
// The picker reads the console itself, rather than letting bubbletea do it,
// because bubbletea cannot tell ctrl+enter from enter. Its key table maps
// the return key to enter no matter what else is held down, so the ctrl bit
// is gone before the picker ever sees the key. The console record still
// carries it, so the picker reads the records and hands the keys over
// itself.
//
// The rest of the mapping is the one bubbletea uses, so the keys behave
// exactly as they did before.

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

// openKeyboard opens the console for the picker to read.
//
// The console is opened for the picker alone. Bubbletea is told there is no
// input at all and the keys are handed to it as they arrive, so the picker
// is the only thing reading the console and no key press is read twice.
//
// A console that cannot be opened is not the end of the road: the picker
// falls back to bubbletea's own reader then, which is what the other builds
// do anyway.
func openKeyboard() keyboard {
	handle, err := openConsole()
	if err != nil {
		return keyboard{}
	}
	mode, err := prepareConsole(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return keyboard{}
	}
	reader := &consoleReader{handle: handle, mode: mode}
	return keyboard{
		options: []tea.ProgramOption{tea.WithInput(nil)},
		start:   reader.serve,
	}
}

// openConsole opens the console input for reading.
//
// The standard input is used when it is a console, which is the usual
// case. When it is not, the console is opened by its name, so the picker
// still reads the keyboard when its input has been redirected.
func openConsole() (windows.Handle, error) {
	if handle, err := coninput.NewStdinHandle(); err == nil {
		var mode uint32
		if windows.GetConsoleMode(handle, &mode) == nil {
			return handle, nil
		}
	}

	name, err := windows.UTF16PtrFromString("CONIN$")
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
}

// prepareConsole puts the console into the mode the picker reads it in.
//
// The window input is wanted, because that is how a resize is reported on
// Windows. The virtual terminal input is not wanted, because it throws the
// modifier keys away and with them the difference between enter and
// ctrl+enter.
//
// The quick edit mode is wanted as well, because it is what lets the mouse
// select the text in the window, so that what the picker shows can be
// copied. The console wants the extended flags with it, and the quick edit
// mode on its own is ignored without them.
//
// The mode that was there before is remembered, so it can be put back when
// the picker is done.
func prepareConsole(handle windows.Handle) (uint32, error) {
	var original uint32
	if err := windows.GetConsoleMode(handle, &original); err != nil {
		return 0, err
	}
	mode := coninput.AddInputModes(0,
		windows.ENABLE_WINDOW_INPUT,
		windows.ENABLE_EXTENDED_FLAGS,
		windows.ENABLE_QUICK_EDIT_MODE,
	)
	if err := windows.SetConsoleMode(handle, mode); err != nil {
		return 0, err
	}
	return original, nil
}

// consoleReader reads the console and hands the keys to the picker.
type consoleReader struct {
	handle windows.Handle
	mode   uint32
}

// serve starts reading and returns the function that stops.
//
// Stopping puts the console mode back and closes the handle, which also
// unblocks a read that is waiting for a key that will never come.
func (c *consoleReader) serve(p *tea.Program) func() {
	stopped := make(chan struct{})
	go c.read(p, stopped)

	var once sync.Once
	return func() {
		once.Do(func() {
			close(stopped)
			_ = windows.SetConsoleMode(c.handle, c.mode)
			_ = windows.CloseHandle(c.handle)
		})
	}
}

// read reads the console until the picker is done with it.
//
// The records are peeked at before they are read, because a console read
// cannot be cancelled reliably and the picker has to be able to stop.
func (c *consoleReader) read(p *tea.Program, stopped chan struct{}) {
	var size coninput.WindowBufferSizeEventRecord

	for {
		select {
		case <-stopped:
			return
		default:
		}

		records, err := coninput.PeekNConsoleInputs(c.handle, 32)
		if err != nil {
			return
		}
		if len(records) == 0 {
			// Nothing to read yet, so wait a moment rather than spin.
			time.Sleep(16 * time.Millisecond)
			continue
		}

		events, err := coninput.ReadNConsoleInputs(c.handle, uint32(len(records)))
		if err != nil {
			return
		}
		for _, event := range events {
			for _, msg := range consoleMessages(event.Unwrap(), &size) {
				p.Send(msg)
			}
		}
	}
}

// consoleMessages turns a console record into the messages the picker
// understands.
//
// A window size is only sent when the size has really changed, so the
// picker is not redrawn for every record of the kind.
func consoleMessages(event coninput.EventRecord, size *coninput.WindowBufferSizeEventRecord) []tea.Msg {
	switch event := event.(type) {
	case coninput.KeyEventRecord:
		if !event.KeyDown || isModifierKey(event.VirtualKeyCode) {
			return nil
		}
		msgs := make([]tea.Msg, 0, event.RepeatCount)
		for i := uint16(0); i < event.RepeatCount; i++ {
			key, ok := keyMessage(event)
			if !ok {
				return nil
			}
			msgs = append(msgs, key)
		}
		return msgs

	case coninput.WindowBufferSizeEventRecord:
		if event == *size {
			return nil
		}
		*size = event
		return []tea.Msg{tea.WindowSizeMsg{
			Width:  int(event.Size.X),
			Height: int(event.Size.Y),
		}}
	}
	return nil
}

// isModifierKey reports whether the key is a modifier on its own.
//
// The console reports a modifier going down as a key press of its own. It
// is not one, and a bare ctrl would otherwise put a null character into the
// commit message.
func isModifierKey(code coninput.VirtualKeyCode) bool {
	switch code {
	case coninput.VK_SHIFT, coninput.VK_CONTROL, coninput.VK_MENU,
		coninput.VK_LWIN, coninput.VK_RWIN, coninput.VK_NUMLOCK,
		coninput.VK_SCROLL:
		return true
	}
	return false
}

// keyMessage turns a console key event into the key the picker understands.
//
// The mapping is the one bubbletea uses for the console, so the keys behave
// exactly as they did before. The one difference is ctrl+enter, which
// bubbletea cannot tell from enter and this can: the console record says
// which modifier keys were held down.
//
// A key with nothing behind it, such as the caps lock, is not a key press
// at all, so it is passed over.
func keyMessage(event coninput.KeyEventRecord) (tea.KeyMsg, bool) {
	code := event.VirtualKeyCode
	leftCtrl := event.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED)
	rightCtrl := event.ControlKeyState.Contains(coninput.RIGHT_CTRL_PRESSED)
	rightAlt := event.ControlKeyState.Contains(coninput.RIGHT_ALT_PRESSED)
	shift := event.ControlKeyState.Contains(coninput.SHIFT_PRESSED)
	alt := event.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED)
	ctrl := leftCtrl || rightCtrl

	// The four kinds of a key that has modifiers: plain, with shift, with
	// ctrl, and with both.
	modified := func(plain, shifted, withCtrl, withBoth tea.KeyType) tea.KeyType {
		switch {
		case shift && ctrl:
			return withBoth
		case shift:
			return shifted
		case ctrl:
			return withCtrl
		default:
			return plain
		}
	}

	switch code {
	case coninput.VK_RETURN:
		if ctrl {
			// Ctrl+Enter finishes the commit message. A terminal sends
			// it as ctrl+j, because it is the same byte, and bubbletea
			// calls that key ctrl+j, so one name covers both.
			return tea.KeyMsg{Type: tea.KeyCtrlJ}, true
		}
		return tea.KeyMsg{Type: tea.KeyEnter}, true
	case coninput.VK_BACK:
		return tea.KeyMsg{Type: tea.KeyBackspace}, true
	case coninput.VK_TAB:
		if shift {
			return tea.KeyMsg{Type: tea.KeyShiftTab}, true
		}
		return tea.KeyMsg{Type: tea.KeyTab}, true
	case coninput.VK_SPACE:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{event.Char}}, true
	case coninput.VK_ESCAPE:
		return tea.KeyMsg{Type: tea.KeyEscape}, true
	case coninput.VK_UP:
		return tea.KeyMsg{Type: modified(tea.KeyUp, tea.KeyShiftUp, tea.KeyCtrlUp, tea.KeyCtrlShiftUp)}, true
	case coninput.VK_DOWN:
		return tea.KeyMsg{Type: modified(tea.KeyDown, tea.KeyShiftDown, tea.KeyCtrlDown, tea.KeyCtrlShiftDown)}, true
	case coninput.VK_RIGHT:
		return tea.KeyMsg{Type: modified(tea.KeyRight, tea.KeyShiftRight, tea.KeyCtrlRight, tea.KeyCtrlShiftRight)}, true
	case coninput.VK_LEFT:
		return tea.KeyMsg{Type: modified(tea.KeyLeft, tea.KeyShiftLeft, tea.KeyCtrlLeft, tea.KeyCtrlShiftLeft)}, true
	case coninput.VK_HOME:
		return tea.KeyMsg{Type: modified(tea.KeyHome, tea.KeyShiftHome, tea.KeyCtrlHome, tea.KeyCtrlShiftHome)}, true
	case coninput.VK_END:
		return tea.KeyMsg{Type: modified(tea.KeyEnd, tea.KeyShiftEnd, tea.KeyCtrlEnd, tea.KeyCtrlShiftEnd)}, true
	case coninput.VK_PRIOR:
		return tea.KeyMsg{Type: tea.KeyPgUp}, true
	case coninput.VK_NEXT:
		return tea.KeyMsg{Type: tea.KeyPgDown}, true
	case coninput.VK_DELETE:
		return tea.KeyMsg{Type: tea.KeyDelete}, true
	case coninput.VK_F1:
		return tea.KeyMsg{Type: tea.KeyF1}, true
	case coninput.VK_F2:
		return tea.KeyMsg{Type: tea.KeyF2}, true
	case coninput.VK_F3:
		return tea.KeyMsg{Type: tea.KeyF3}, true
	case coninput.VK_F4:
		return tea.KeyMsg{Type: tea.KeyF4}, true
	case coninput.VK_F5:
		return tea.KeyMsg{Type: tea.KeyF5}, true
	case coninput.VK_F6:
		return tea.KeyMsg{Type: tea.KeyF6}, true
	case coninput.VK_F7:
		return tea.KeyMsg{Type: tea.KeyF7}, true
	case coninput.VK_F8:
		return tea.KeyMsg{Type: tea.KeyF8}, true
	case coninput.VK_F9:
		return tea.KeyMsg{Type: tea.KeyF9}, true
	case coninput.VK_F10:
		return tea.KeyMsg{Type: tea.KeyF10}, true
	case coninput.VK_F11:
		return tea.KeyMsg{Type: tea.KeyF11}, true
	case coninput.VK_F12:
		return tea.KeyMsg{Type: tea.KeyF12}, true
	}

	// The rest are the characters and the control keys they stand for.
	//
	// AltGr holds ctrl and alt together and produces ordinary characters,
	// so a key with both is a character and not a control key.
	if !(leftCtrl && rightAlt) && ctrl {
		switch event.Char {
		case 0x00:
			return tea.KeyMsg{Type: tea.KeyCtrlAt}, true
		case 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
			0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
			0x15, 0x16, 0x17, 0x18, 0x19, 0x1a:
			// The control characters that stand for a letter.
			return tea.KeyMsg{Type: tea.KeyCtrlA + tea.KeyType(event.Char-0x01)}, true
		case 0x1b:
			return tea.KeyMsg{Type: tea.KeyCtrlOpenBracket}, true
		case 0x1c:
			return tea.KeyMsg{Type: tea.KeyCtrlBackslash}, true
		case 0x1f:
			return tea.KeyMsg{Type: tea.KeyCtrlUnderscore}, true
		}
		switch code {
		case coninput.VK_OEM_4:
			return tea.KeyMsg{Type: tea.KeyCtrlOpenBracket}, true
		case coninput.VK_OEM_6:
			return tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}, true
		}
		// A control key with no character behind it still has a virtual
		// key code, and that is enough to tell which letter it was.
		if code >= 0x41 && code <= 0x5a {
			return tea.KeyMsg{Type: tea.KeyCtrlA + tea.KeyType(code-0x41)}, true
		}
	}

	if event.Char == 0 {
		// There is no character behind this key, so it is not one.
		return tea.KeyMsg{}, false
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{event.Char}, Alt: alt}, true
}
