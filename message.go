package main

// The commit message the user types.
//
// The message is one string with the line breaks in it, and the caret is a
// place in that string, so a message of more than one line needs nothing
// more than a single line of state.

import "strings"

// messageLines splits the message into the lines it is made of.
func messageLines(text string) []string {
	return strings.Split(text, "\n")
}

// caret is where the caret is in the message: the line it is on and how far
// along that line it is.
type caret struct {
	line   int
	column int
}

// caretPlace works out where the caret is in the message.
//
// The place is counted in characters and not in bytes, so a message with
// anything outside ASCII in it still puts the caret in the right spot.
func caretPlace(text string, pos int) caret {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	place := caret{}
	for i := 0; i < pos; i++ {
		if runes[i] == '\n' {
			place.line++
			place.column = 0
			continue
		}
		place.column++
	}
	return place
}

// lineStart is the place in the message where a line begins.
func lineStart(text string, line int) int {
	if line <= 0 {
		return 0
	}
	runes := []rune(text)
	count := 0
	for i, r := range runes {
		if r == '\n' {
			count++
			if count == line {
				return i + 1
			}
		}
	}
	return len(runes)
}

// moveCaretLine moves the caret a line up or down.
//
// The caret keeps its column where the line is long enough for it and goes
// to the end of the line where it is not, which is what any editor does.
// A caret that is already on the first or the last line stays where it is.
func moveCaretLine(text string, pos, step int) int {
	place := caretPlace(text, pos)
	lines := messageLines(text)
	target := place.line + step
	if target < 0 || target >= len(lines) {
		return pos
	}

	column := place.column
	width := len([]rune(lines[target]))
	if column > width {
		column = width
	}
	return lineStart(text, target) + column
}

// insertText puts text into the message at the caret.
//
// The new place of the caret is worked out as well, so the caret stays
// behind what was typed.
func insertText(text string, pos int, insert string) (string, int) {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	out := string(runes[:pos]) + insert + string(runes[pos:])
	return out, pos + len([]rune(insert))
}

// deleteBack removes the character before the caret.
//
// At the start of a line it removes the line break, so the two lines are
// joined, which is what backspace does in any editor.
func deleteBack(text string, pos int) (string, int) {
	runes := []rune(text)
	if pos <= 0 {
		return text, 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	return string(runes[:pos-1]) + string(runes[pos:]), pos - 1
}

// messageScroll is the first line the box shows.
//
// The box follows the caret, so the line the caret is on is always on
// screen. A message that fits needs no scrolling at all.
func messageScroll(lines []string, caretLine, rows int) int {
	if len(lines) <= rows {
		return 0
	}
	start := caretLine - rows + 1
	if start < 0 {
		start = 0
	}
	if start > len(lines)-rows {
		start = len(lines) - rows
	}
	return start
}
