//go:build windows

package main

// Tests for the console reader.
//
// The console says which modifier keys were held down, and the reader turns
// that into the keys the picker understands.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
)

// keyEvent builds the record the console sends for a key press.
func keyEvent(code coninput.VirtualKeyCode, char rune, state coninput.ControlKeyState) coninput.KeyEventRecord {
	return coninput.KeyEventRecord{
		KeyDown:         true,
		VirtualKeyCode:  code,
		Char:            char,
		ControlKeyState: state,
	}
}

func TestCtrlEnterIsToldApartFromEnter(t *testing.T) {
	// The console holds the ctrl bit in the record, so ctrl+enter is a
	// key of its own and not the same key as enter.
	ctrl := keyEvent(coninput.VK_RETURN, '\n', coninput.LEFT_CTRL_PRESSED)
	key, ok := keyMessage(ctrl)
	if !ok {
		t.Fatal("ctrl+enter should be a key")
	}
	if key.Type != tea.KeyCtrlJ {
		t.Fatalf("ctrl+enter: got type %d, want %d", key.Type, tea.KeyCtrlJ)
	}
	if key.String() != "ctrl+j" {
		t.Fatalf("ctrl+enter should be the ctrl+j key, got %q", key.String())
	}

	// Plain enter is still enter.
	plain := keyEvent(coninput.VK_RETURN, '\r', 0)
	key, ok = keyMessage(plain)
	if !ok {
		t.Fatal("enter should be a key")
	}
	if key.Type != tea.KeyEnter {
		t.Fatalf("enter: got type %d, want %d", key.Type, tea.KeyEnter)
	}
}

func TestTheControlKeysStillWork(t *testing.T) {
	// The keys the picker uses the most have to come out of the reader
	// the way they always did.
	cases := []struct {
		name string
		code coninput.VirtualKeyCode
		char rune
		want tea.KeyType
	}{
		{"ctrl+c", coninput.VK_C, '\x03', tea.KeyCtrlC},
		{"ctrl+g", coninput.VK_G, '\a', tea.KeyCtrlG},
		{"ctrl+n", coninput.VK_N, '\x0e', tea.KeyCtrlN},
		{"ctrl+p", coninput.VK_P, '\x10', tea.KeyCtrlP},
		{"ctrl+s", coninput.VK_S, '\x13', tea.KeyCtrlS},
		{"esc", coninput.VK_ESCAPE, '\x1b', tea.KeyEscape},
		{"backspace", coninput.VK_BACK, '\b', tea.KeyBackspace},
		{"up", coninput.VK_UP, 0, tea.KeyUp},
		{"down", coninput.VK_DOWN, 0, tea.KeyDown},
		{"left", coninput.VK_LEFT, 0, tea.KeyLeft},
		{"right", coninput.VK_RIGHT, 0, tea.KeyRight},
		{"pgup", coninput.VK_PRIOR, 0, tea.KeyPgUp},
		{"pgdn", coninput.VK_NEXT, 0, tea.KeyPgDown},
	}

	for _, c := range cases {
		state := coninput.ControlKeyState(0)
		if len(c.name) > 4 && c.name[:4] == "ctrl" {
			state = coninput.LEFT_CTRL_PRESSED
		}
		key, ok := keyMessage(keyEvent(c.code, c.char, state))
		if !ok {
			t.Fatalf("%s should be a key", c.name)
		}
		if key.Type != c.want {
			t.Fatalf("%s: got type %d, want %d", c.name, key.Type, c.want)
		}
	}
}

func TestALetterIsACharacter(t *testing.T) {
	key, ok := keyMessage(keyEvent(coninput.VK_A, 'a', 0))
	if !ok {
		t.Fatal("a letter should be a key")
	}
	if key.Type != tea.KeyRunes {
		t.Fatalf("a letter: got type %d, want %d", key.Type, tea.KeyRunes)
	}
	if string(key.Runes) != "a" {
		t.Fatalf("a letter: got %q", string(key.Runes))
	}
}

func TestTheSpaceIsACharacter(t *testing.T) {
	key, ok := keyMessage(keyEvent(coninput.VK_SPACE, ' ', 0))
	if !ok {
		t.Fatal("the space should be a key")
	}
	if key.Type != tea.KeyRunes || string(key.Runes) != " " {
		t.Fatalf("the space: got type %d runes %q", key.Type, string(key.Runes))
	}
}

func TestAModifierOnItsOwnIsNotAKey(t *testing.T) {
	// The console reports a modifier going down as a key press of its
	// own, and a bare ctrl would put a null character into the message.
	for _, code := range []coninput.VirtualKeyCode{
		coninput.VK_SHIFT, coninput.VK_CONTROL, coninput.VK_MENU,
	} {
		if !isModifierKey(code) {
			t.Fatalf("the modifier %d should be one", code)
		}
		if _, ok := keyMessage(keyEvent(code, 0, 0)); ok {
			t.Fatalf("the modifier %d should not be a key", code)
		}
	}
}

func TestAKeyWithNothingBehindItIsNotAKey(t *testing.T) {
	// The F13 key is not one the picker knows, and it has no character
	// behind it either, so it is not a key press.
	if _, ok := keyMessage(keyEvent(0x7c, 0, 0)); ok {
		t.Fatal("a key with no character should not be a key")
	}
}

func TestTheWindowSizeIsSentWhenItChanges(t *testing.T) {
	var size coninput.WindowBufferSizeEventRecord

	messages := consoleMessages(coninput.WindowBufferSizeEventRecord{
		Size: coninput.Coord{X: 120, Y: 40},
	}, &size)
	if len(messages) != 1 {
		t.Fatalf("the size should be sent once, got %d messages", len(messages))
	}
	resize, ok := messages[0].(tea.WindowSizeMsg)
	if !ok {
		t.Fatalf("the message should be a resize, got %T", messages[0])
	}
	if resize.Width != 120 || resize.Height != 40 {
		t.Fatalf("the size: got %dx%d", resize.Width, resize.Height)
	}

	// The same size again is not worth a redraw.
	messages = consoleMessages(coninput.WindowBufferSizeEventRecord{
		Size: coninput.Coord{X: 120, Y: 40},
	}, &size)
	if len(messages) != 0 {
		t.Fatalf("the same size should not be sent again, got %d messages", len(messages))
	}
}
