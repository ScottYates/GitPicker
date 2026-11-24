package main

// The help panel.
//
// The branch list shows a hint at the bottom with the keys that matter most,
// and this panel holds every key there is, with a line of explanation for
// each. The question mark opens it.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// helpKey is a key and what it does.
type helpKey struct {
	key  string
	what string
}

// helpSection is a group of keys with a heading.
type helpSection struct {
	title string
	keys  []helpKey
}

// helpSections are the keys of the picker, grouped by where they are used.
var helpSections = []helpSection{
	{
		title: "the branch list",
		keys: []helpKey{
			{"type", "filter the branches"},
			{"up/down", "move the pointer"},
			{"pgup/pgdn", "move a page"},
			{"enter", "switch to the branch"},
			{"ctrl+s", "change the sort"},
			{"ctrl+n", "make a new branch"},
			{"ctrl+c", "commit"},
			{"ctrl+p", "push"},
			{"ctrl+g", "clone into this folder"},
			{"ctrl+o", "show or hide what git said"},
			{"ctrl+u", "update to the newest build"},
			{"?", "this help"},
			{"esc", "leave"},
		},
	},
	{
		title: "the commit",
		keys: []helpKey{
			{"space", "tick a file"},
			{"enter", "add a line to the message"},
			{"ctrl+enter", "finish the message and commit"},
			{"ctrl+p/ctrl+n", "the messages you wrote before"},
			{"esc", "go back"},
		},
	},
}

// helpKeyWidth is the width of the key column in the help panel.
const helpKeyWidth = 16

// helpLine is one line of the help panel.
type helpLine struct {
	text string

	// title is true for the heading of a section.
	title bool
}

// helpLines is the whole of the help panel.
func helpLines() []helpLine {
	var lines []helpLine
	for i, section := range helpSections {
		if i > 0 {
			lines = append(lines, helpLine{})
		}
		lines = append(lines, helpLine{text: section.title, title: true})
		for _, entry := range section.keys {
			text := padToWidth(entry.key, helpKeyWidth) + entry.what
			lines = append(lines, helpLine{text: text})
		}
	}
	return lines
}

// helpRows is how many lines of help fit on the screen.
//
// The panel has a heading, a rule above and below it and a hint under it.
func (m model) helpRows() int {
	rows := m.height - 5
	if rows < 1 {
		rows = 1
	}
	return rows
}

// handleHelpKey acts on a key press while the help is on screen.
//
// The help is read and then left, so escape and enter close it. The console
// pastes the clipboard when the mouse is clicked in the window, so a stray
// click should not close it either.
func (m model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(msg) {
	case "esc", "enter":
		m.phase = phaseList
		return m, nil
	case "up":
		if m.helpScroll > 0 {
			m.helpScroll--
		}
		return m, nil
	case "down":
		m.helpScroll++
		if max := len(helpLines()) - m.helpRows(); m.helpScroll > max {
			m.helpScroll = max
		}
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
		return m, nil
	case "pgup":
		m.helpScroll -= m.helpRows()
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
		return m, nil
	case "pgdn":
		m.helpScroll += m.helpRows()
		if max := len(helpLines()) - m.helpRows(); m.helpScroll > max {
			m.helpScroll = max
		}
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
		return m, nil
	}
	return m, nil
}

// viewHelp draws the panel with every key in it.
func (m model) viewHelp() string {
	var b strings.Builder

	b.WriteString("  " + "keys" + "\n\n")

	width := m.panelWidth()
	lines := helpLines()
	rows := m.helpRows()

	start := m.helpScroll
	if start > len(lines) {
		start = len(lines)
	}
	if start < 0 {
		start = 0
	}
	end := start + rows
	if end > len(lines) {
		end = len(lines)
	}

	b.WriteString(panelRule(width, "keys") + "\n")
	for _, line := range lines[start:end] {
		text := line.text
		if line.title {
			text = styleTitle.Render(text)
		}
		b.WriteString(styleBorder.Render(vertical+" ") + padToWidth(text, width-2) + "\n")
	}
	b.WriteString(panelBottom(width) + "\n")

	position := fmt.Sprintf("%d of %d", end, len(lines))
	b.WriteString(styleInfo.Render("esc closes the help, arrows scroll (" + position + ")"))
	return b.String()
}
