package main

// Tests for the commit message as text: where the caret is, what a line
// break does, and how the box scrolls.

import "testing"

func TestCaretPlaceFindsTheLineAndTheColumn(t *testing.T) {
	text := "first\nsecond\nthird"

	place := caretPlace(text, 0)
	if place.line != 0 || place.column != 0 {
		t.Fatalf("the start: got line %d column %d", place.line, place.column)
	}

	place = caretPlace(text, 8)
	if place.line != 1 || place.column != 2 {
		t.Fatalf("the middle: got line %d column %d", place.line, place.column)
	}

	place = caretPlace(text, len([]rune(text)))
	if place.line != 2 || place.column != 5 {
		t.Fatalf("the end: got line %d column %d", place.line, place.column)
	}
}

func TestCaretPlaceCountsCharactersAndNotBytes(t *testing.T) {
	// The message is allowed to hold anything, so the caret has to be
	// placed in characters and not in bytes. The e with the accent is
	// written as an escape, so this file stays plain ASCII.
	text := "caf\u00e9\nnext"

	place := caretPlace(text, 4)
	if place.line != 0 || place.column != 4 {
		t.Fatalf("before the line break: got line %d column %d", place.line, place.column)
	}

	place = caretPlace(text, 5)
	if place.line != 1 || place.column != 0 {
		t.Fatalf("after the line break: got line %d column %d", place.line, place.column)
	}
}

func TestLineStartFindsWhereALineBegins(t *testing.T) {
	text := "first\nsecond\nthird"

	if got := lineStart(text, 0); got != 0 {
		t.Fatalf("the first line: got %d", got)
	}
	if got := lineStart(text, 1); got != 6 {
		t.Fatalf("the second line: got %d", got)
	}
	if got := lineStart(text, 2); got != 13 {
		t.Fatalf("the third line: got %d", got)
	}
}

func TestMoveCaretLineKeepsTheColumn(t *testing.T) {
	text := "first\nsecond"

	// Down from the third character of the first line.
	got := moveCaretLine(text, 3, 1)
	if got != 9 {
		t.Fatalf("down: got %d, want 9", got)
	}

	// And back up again.
	got = moveCaretLine(text, got, -1)
	if got != 3 {
		t.Fatalf("up: got %d, want 3", got)
	}
}

func TestMoveCaretLineStopsAtTheEndOfAShortLine(t *testing.T) {
	text := "a long line\nab"

	// The second line is too short for the column the caret was in, so
	// the caret goes to the end of it.
	got := moveCaretLine(text, 5, 1)
	if got != 14 {
		t.Fatalf("down: got %d, want 14", got)
	}
}

func TestMoveCaretLineStaysAtTheEnds(t *testing.T) {
	text := "first\nsecond"

	if got := moveCaretLine(text, 2, -1); got != 2 {
		t.Fatalf("up from the first line: got %d, want 2", got)
	}
	if got := moveCaretLine(text, 8, 1); got != 8 {
		t.Fatalf("down from the last line: got %d, want 8", got)
	}
}

func TestInsertTextPutsTheCaretBehindWhatWasTyped(t *testing.T) {
	text, pos := insertText("first", 5, " and more")
	if text != "first and more" {
		t.Fatalf("the message: got %q", text)
	}
	if pos != 14 {
		t.Fatalf("the caret: got %d, want 14", pos)
	}

	// And in the middle of what is there.
	text, pos = insertText("first", 0, ">> ")
	if text != ">> first" {
		t.Fatalf("the message: got %q", text)
	}
	if pos != 3 {
		t.Fatalf("the caret: got %d, want 3", pos)
	}
}

func TestDeleteBackJoinsTwoLines(t *testing.T) {
	// Backspace at the start of a line takes the line break away, so the
	// two lines become one.
	text, pos := deleteBack("first\nsecond", 6)
	if text != "firstsecond" {
		t.Fatalf("the message: got %q", text)
	}
	if pos != 5 {
		t.Fatalf("the caret: got %d, want 5", pos)
	}

	// And at the very start there is nothing to delete.
	text, pos = deleteBack("first", 0)
	if text != "first" {
		t.Fatalf("the message: got %q", text)
	}
	if pos != 0 {
		t.Fatalf("the caret: got %d, want 0", pos)
	}
}

func TestMessageScrollKeepsTheCaretOnScreen(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}

	// A message that fits needs no scrolling.
	if got := messageScroll(lines[:2], 1, 5); got != 0 {
		t.Fatalf("a message that fits: got %d, want 0", got)
	}

	// The caret on the last line means the last lines are shown.
	got := messageScroll(lines, 9, 4)
	if got != 6 {
		t.Fatalf("the caret on the last line: got %d, want 6", got)
	}

	// And the caret on the first line means the first ones are shown.
	got = messageScroll(lines, 0, 4)
	if got != 0 {
		t.Fatalf("the caret on the first line: got %d, want 0", got)
	}

	// A caret in the middle shows the lines around it.
	got = messageScroll(lines, 5, 4)
	if got != 2 {
		t.Fatalf("the caret in the middle: got %d, want 2", got)
	}
}
