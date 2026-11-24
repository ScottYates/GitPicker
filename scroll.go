package main

// The scrolling of the text panels.
//
// The output panel, the help panel and the error panel all show more text
// than fits on the screen. They scroll the same way: a line at a time with
// the arrows and a screenful at a time with the page keys.
//
// The position is never past the end of the text. A panel scrolled past the
// end would have nothing on it at all, which looks like the picker has
// stopped working rather than like the reader is at the bottom.

import "fmt"

// clampScroll keeps a scroll position inside the text.
//
// The position is the line that is at the top of the panel, so the last page
// that still has something to show starts at total-rows.
func clampScroll(offset, total, rows int) int {
	if rows < 1 {
		rows = 1
	}
	last := total - rows
	if last < 0 {
		last = 0
	}
	if offset > last {
		offset = last
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// scrollPosition says where the reader is in the text.
//
// It is the line at the top of the panel. The end of the text is said as
// such, so the last page is not mistaken for a page in the middle.
func scrollPosition(offset, total, rows int) string {
	if total == 0 {
		return "end of 0"
	}
	if offset+rows >= total {
		return fmt.Sprintf("end of %d", total)
	}
	return fmt.Sprintf("%d of %d", offset+1, total)
}
