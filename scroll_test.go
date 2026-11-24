package main

// Tests for the text panels that scroll: the output panel, the error panel
// and the current branch at the top of the list.

import (
	"strings"
	"testing"
)

func TestClampScrollKeepsThePositionInsideTheText(t *testing.T) {
	cases := []struct {
		name   string
		offset int
		total  int
		rows   int
		want   int
	}{
		{"at the top", 0, 40, 10, 0},
		{"in the middle", 12, 40, 10, 12},
		{"past the end", 39, 40, 10, 30},
		{"before the top", -5, 40, 10, 0},
		{"shorter than the panel", 5, 4, 10, 0},
		{"no rows at all", 3, 40, 0, 3},
	}
	for _, c := range cases {
		if got := clampScroll(c.offset, c.total, c.rows); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestScrollPositionSaysWhereTheReaderIs(t *testing.T) {
	if got := scrollPosition(0, 40, 10); got != "1 of 40" {
		t.Fatalf("the position should be the first line, got %q", got)
	}
	if got := scrollPosition(30, 40, 10); got != "end of 40" {
		t.Fatalf("the last page should say so, got %q", got)
	}
}

func TestPageDownScrollsTheOutput(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 10
	m.phase = phaseOutput
	m.output = strings.Repeat("a line of output\n", 40)

	next, _ := m.handleKey(keyMsg("pgdown"))
	m = next.(model)

	if m.outputScroll != m.outputRows() {
		t.Fatalf("a page down should have moved a screenful, got %d", m.outputScroll)
	}

	// The panel still has text on it.
	panel := visible(m.View())
	if !strings.Contains(panel, "a line of output") {
		t.Fatalf("the panel should still show the output:\n%s", panel)
	}
}

func TestPageDownStopsAtTheEndOfTheOutput(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 10
	m.phase = phaseOutput
	m.output = strings.Repeat("a line of output\n", 40)

	rows := m.outputRows()
	for i := 0; i < 20; i++ {
		next, _ := m.handleKey(keyMsg("pgdown"))
		m = next.(model)
	}

	// The last page shows the last line of the text at the bottom, so the
	// scroll stops there rather than running past it.
	lines := m.outputLines(m.panelWidth() - 2)
	want := len(lines) - rows
	if m.outputScroll != want {
		t.Fatalf("the scroll should have stopped at %d, got %d", want, m.outputScroll)
	}

	// A panel scrolled past the end would be empty, which looks like the
	// picker has hung.
	panel := visible(m.View())
	if !strings.Contains(panel, "a line of output") {
		t.Fatalf("the panel should still show the output:\n%s", panel)
	}
}

func TestPageUpGoesBackToTheTopOfTheOutput(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 10
	m.phase = phaseOutput
	m.output = strings.Repeat("a line of output\n", 40)

	next, _ := m.handleKey(keyMsg("pgdown"))
	m = next.(model)
	next, _ = m.handleKey(keyMsg("pgup"))
	m = next.(model)

	if m.outputScroll != 0 {
		t.Fatalf("a page up should have gone back to the top, got %d", m.outputScroll)
	}
}

func TestTheErrorPanelScrolls(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 10
	m.phase = phaseError
	m.fatal = &GitError{Code: 1, Message: "git pull failed", Detail: strings.Repeat("a line of the error\n", 40)}

	next, _ := m.handleKey(keyMsg("pgdown"))
	m = next.(model)

	if m.errorScroll != m.errorRows() {
		t.Fatalf("a page down should have moved a screenful, got %d", m.errorScroll)
	}

	panel := visible(m.View())
	if !strings.Contains(panel, "a line of the error") {
		t.Fatalf("the panel should still show the error:\n%s", panel)
	}

	// The end of the error is reachable and the panel is not empty there.
	for i := 0; i < 20; i++ {
		next, _ = m.handleKey(keyMsg("pgdown"))
		m = next.(model)
	}
	lines := m.errorLines(m.panelWidth() - 2)
	if m.errorScroll != len(lines)-m.errorRows() {
		t.Fatalf("the scroll should have stopped at the end, got %d", m.errorScroll)
	}
	panel = visible(m.View())
	if !strings.Contains(panel, "a line of the error") {
		t.Fatalf("the panel should still show the error:\n%s", panel)
	}
}

func TestTheErrorPanelKeepsTheWholeText(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 10
	m.phase = phaseError
	m.fatal = &GitError{Code: 1, Message: "git pull failed", Detail: strings.Repeat("a line of the error\n", 40)}

	// Nothing is left out of the text any more, because the panel scrolls.
	lines := m.errorLines(68)
	if len(lines) != 41 {
		t.Fatalf("the whole error should be there, got %d lines", len(lines))
	}
	if strings.Contains(visible(m.View()), "more lines, see the terminal") {
		t.Fatalf("the note about the rest should be gone:\n%s", visible(m.View()))
	}
}

func TestTheCurrentBranchIsAtTheTop(t *testing.T) {
	m := testModel("master", "feature/x", "release/1")
	m.width = 80
	m.height = 20

	if m.branchAt(m.matches[0]).Name != "master" {
		t.Fatalf("the current branch should be first, got %q", m.branchAt(m.matches[0]).Name)
	}
	// The pointer is on it, so the reader starts where they are.
	if m.branchAt(m.matches[m.cursor]).Name != "master" {
		t.Fatalf("the pointer should be on the current branch, got %q", m.branchAt(m.matches[m.cursor]).Name)
	}
}

func TestTheCurrentBranchIsAtTheTopWhateverTheSort(t *testing.T) {
	m := testModel("master", "feature/x", "release/1")
	m.width = 80
	m.height = 20
	setCurrent(&m, "release/1")

	m.toggleSort()
	if m.branchAt(m.matches[0]).Name != "release/1" {
		t.Fatalf("the current branch should be first by name, got %q", m.branchAt(m.matches[0]).Name)
	}

	m.toggleSort()
	if m.branchAt(m.matches[0]).Name != "release/1" {
		t.Fatalf("the current branch should be first by time, got %q", m.branchAt(m.matches[0]).Name)
	}
}

func TestTheCurrentBranchStaysAtTheTopWhileTheQueryMatches(t *testing.T) {
	m := testModel("master", "feature/x", "release/1")
	m.width = 80
	m.height = 20
	setCurrent(&m, "feature/x")

	m.query = "e"
	m.refilter()
	if m.branchAt(m.matches[0]).Name != "feature/x" {
		t.Fatalf("the current branch should be first, got %q", m.branchAt(m.matches[0]).Name)
	}
}

func TestTheCurrentBranchGoesWhenTheQueryDoesNotMatchIt(t *testing.T) {
	m := testModel("master", "feature/x", "release/1")
	m.width = 80
	m.height = 20
	setCurrent(&m, "feature/x")

	m.query = "master"
	m.refilter()
	for _, entry := range m.matches {
		if m.branchAt(entry).Name == "feature/x" {
			t.Fatalf("the current branch should not be in the list")
		}
	}
}

func TestThePointerStartsOnTheCurrentBranch(t *testing.T) {
	m := testModel("main", "master", "feature/x")
	m.width = 80
	m.height = 20
	setCurrent(&m, "feature/x")

	view := visible(m.View())
	if !strings.Contains(view, pointer+" * feature/x") {
		t.Fatalf("the pointer should be on the current branch:\n%s", view)
	}
}

// setCurrent changes the branch that is checked out and rebuilds the list,
// the way the picker does after a switch.
func setCurrent(m *model, name string) {
	m.current = name
	m.rebuildLabels()
	m.refilter()
}
