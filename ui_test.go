package main

// Tests for the picker itself: paging, the sort toggle, the command panel
// and the row rendering.

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg builds the key press a test wants to send.
func keyMsg(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyCtrlP}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "ctrl+enter":
		// Ctrl+Enter is the same byte as ctrl+j, which is the name
		// bubbletea gives it.
		return tea.KeyMsg{Type: tea.KeyCtrlJ}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// runCmd runs a command and returns the message it produced.
//
// The picker runs the git command and the spinner together, so the command
// is a batch and the message has to be looked for inside it.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("there was no command to run")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		result := c()
		if _, isBatch := result.(tea.BatchMsg); isBatch {
			continue
		}
		return result
	}
	return nil
}

// ansiPattern matches the escape sequences lipgloss adds, so a rendered
// string can be measured in visible characters.
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visible strips the styling so the printed width can be checked.
func visible(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

// testModel builds a picker over a fixed set of branches.
func testModel(names ...string) model {
	branches := make([]Branch, 0, len(names))
	for _, name := range names {
		branches = append(branches, Branch{Name: name, Timestamp: testNow})
	}
	ch := make(chan fetchResult, 1)
	return newModel(`C:\repo`, "repo", "master", branches, ch, nil)
}

func TestPagingClampsAtBothEnds(t *testing.T) {
	names := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		names = append(names, string(rune('a'+i%26))+strings.Repeat("x", i))
	}
	m := testModel(names...)
	m.height = 24

	// A page down from the top lands one screenful down.
	m.movePage(1)
	if m.cursor != m.pageRows() {
		t.Fatalf("after a page down: got cursor %d, want %d", m.cursor, m.pageRows())
	}

	// Paging past the end stops at the last item rather than wrapping.
	m.movePage(10)
	if m.cursor != len(m.matches)-1 {
		t.Fatalf("paging past the end: got cursor %d, want %d", m.cursor, len(m.matches)-1)
	}

	// And paging back before the start stops at the top.
	m.cursor = 0
	m.movePage(-1)
	if m.cursor != 0 {
		t.Fatalf("paging before the start: got cursor %d, want 0", m.cursor)
	}
}

func TestPagingKeepsThePointerVisible(t *testing.T) {
	m := testModel("a", "b", "c", "d", "e")
	m.height = 24
	m.cursor = 4
	m.scrollIntoView()
	if m.offset > m.cursor {
		t.Fatalf("the pointer scrolled out of view: offset %d, cursor %d", m.offset, m.cursor)
	}
}

func TestSortToggleKeepsTheSelection(t *testing.T) {
	m := testModel("zulu", "master", "alpha")
	m.cursor = 1
	selected := m.branchAt(m.matches[m.cursor]).Name

	m.toggleSort()
	if m.sortMode != sortName {
		t.Fatalf("sort mode: got %q, want %q", m.sortMode, sortName)
	}
	if got := m.branchAt(m.matches[m.cursor]).Name; got != selected {
		t.Fatalf("the selection moved: got %q, want %q", got, selected)
	}

	m.toggleSort()
	if m.sortMode != sortTime {
		t.Fatalf("sort mode: got %q, want %q", m.sortMode, sortTime)
	}
	if got := m.branchAt(m.matches[m.cursor]).Name; got != selected {
		t.Fatalf("the selection moved: got %q, want %q", got, selected)
	}
}

func TestSortToggleKeepsPinsFirst(t *testing.T) {
	m := testModel("aaa", "master", "zzz")
	m.toggleSort()
	if got := m.branchAt(m.matches[0]).Name; got != "master" {
		t.Fatalf("the pinned branch should still lead, got %q", got)
	}
}

func TestToggleSortReordersTheList(t *testing.T) {
	m := testModel("zulu", "alpha", "mike")
	m.toggleSort()
	got := []string{}
	for _, entry := range m.matches {
		got = append(got, m.branchAt(entry).Name)
	}
	want := []string{"alpha", "mike", "zulu"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("alphabetical order: got %v, want %v", got, want)
		}
	}
}

func TestTheSpinnerShowsWhileGitIsWorking(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 24
	m.status = "switching to master..."
	m.busy = true

	view := visible(m.View())
	if !strings.Contains(view, "| switching to master...") {
		t.Fatalf("the spinner should be in the status line:\n%s", view)
	}

	// And it is gone once the work is done.
	m.busy = false
	view = visible(m.View())
	if strings.Contains(view, "| switching to master...") {
		t.Fatalf("the spinner should be gone when git is done:\n%s", view)
	}
}

func TestTheSpinnerGoesOnAndStops(t *testing.T) {
	m := testModel("master")
	m.busy = true

	next, cmd := m.Update(spinnerTickMsg{})
	m = next.(model)
	if m.spinnerFrame != 1 {
		t.Fatalf("the spinner should have moved on, got frame %d", m.spinnerFrame)
	}
	if cmd == nil {
		t.Fatal("the spinner should keep going while git works")
	}

	// The frames go round and round.
	next, _ = m.Update(spinnerTickMsg{})
	m = next.(model)
	next, _ = m.Update(spinnerTickMsg{})
	m = next.(model)
	next, _ = m.Update(spinnerTickMsg{})
	m = next.(model)
	if m.spinnerFrame != 0 {
		t.Fatalf("the spinner should have come round, got frame %d", m.spinnerFrame)
	}

	// And it stops when git is done.
	m.busy = false
	next, cmd = m.Update(spinnerTickMsg{})
	m = next.(model)
	if cmd != nil {
		t.Fatal("the spinner should stop when git is done")
	}
}

func TestPanelShowsTheCommands(t *testing.T) {
	m := testModel("master", "feature/x")
	m.width = 80
	m.height = 24
	m.branches[0].Upstream = "origin/master"

	lines := m.panelLines()
	if len(lines) != 3 {
		t.Fatalf("the panel should be three rows, got %d", len(lines))
	}

	title := visible(lines[0])
	if !strings.HasPrefix(title, "+-- will run ") {
		t.Fatalf("panel title: got %q", title)
	}
	if len(title) != 80 {
		t.Fatalf("the panel should fill the width: got %d cells", len(title))
	}

	body := visible(lines[1])
	if !strings.HasPrefix(body, "| git switch master") {
		t.Fatalf("panel body: got %q", body)
	}
	if len(body) != 80 {
		t.Fatalf("the panel body should fill the width: got %d cells", len(body))
	}
}

func TestPanelWarnsWhenThereIsNoUpstream(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 24

	lines := m.panelLines()
	if len(lines) < 3 {
		t.Fatalf("the panel should be three rows, got %d", len(lines))
	}
	if !strings.Contains(visible(lines[2]), "no upstream") {
		t.Fatalf("the panel should warn about the missing upstream: got %q", visible(lines[2]))
	}
}

func TestPanelHasAWidthFloor(t *testing.T) {
	m := testModel("master")
	m.width = 10
	m.height = 24

	lines := m.panelLines()
	if len(visible(lines[0])) < minPanelWidth {
		t.Fatalf("the panel should not go below %d cells, got %d", minPanelWidth, len(visible(lines[0])))
	}
}

func TestStyledLabelKeepsItsWidth(t *testing.T) {
	m := testModel("master", "feature/x")
	m.width = 80
	m.height = 24

	label := m.labels[0]
	styled := styleLabel(m.branchAt(m.matches[0]).Name, label, []int{2, 3}, true)
	if len(visible(styled)) != len(label) {
		t.Fatalf("styling changed the width: got %d, want %d", len(visible(styled)), len(label))
	}
}

func TestViewFitsTheTerminalHeight(t *testing.T) {
	m := testModel("master", "feature/x", "release/1")
	m.width = 80
	m.height = 20

	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("the view should fill the screen: got %d rows, want %d", len(lines), m.height)
	}
}

func TestViewShowsTheRepositoryName(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	if !strings.Contains(m.View(), "repo") {
		t.Fatal("the repository name should be in the header")
	}
}

func TestViewShowsThePosition(t *testing.T) {
	m := testModel("master", "feature/x")
	m.width = 80
	m.height = 20

	if !strings.Contains(visible(m.View()), "[1/2]") {
		t.Fatalf("the position readout is missing: %q", visible(m.View()))
	}
}

func TestViewShowsTheSortMode(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	if !strings.Contains(visible(m.View()), "sort: time") {
		t.Fatalf("the sort mode is missing: %q", visible(m.View()))
	}
}

func TestQueryFiltersAndReportsTheCount(t *testing.T) {
	m := testModel("master", "feature/x", "release/1")
	m.width = 80
	m.height = 20
	m.query = "master"
	m.refilter()

	view := visible(m.View())
	if !strings.Contains(view, "(1 of 3 match)") {
		t.Fatalf("the match count is missing: %q", view)
	}
}

func TestNewBranchPromptShowsBothCommands(t *testing.T) {
	m := testModel("master", "feature/x")
	m.width = 80
	m.height = 20
	m.phase = phaseName
	m.nameBuf = "my-branch"

	view := visible(m.View())
	if !strings.Contains(view, "git switch master") {
		t.Fatalf("the switch command is missing: %q", view)
	}
	if !strings.Contains(view, "git switch -c my-branch") {
		t.Fatalf("the create command is missing: %q", view)
	}
}

func TestValidNameStartsTheWork(t *testing.T) {
	m := testModel("master", "feature/x")
	m.phase = phaseName
	m.nameBuf = " my-branch "
	next, cmd := m.handleNameKey(keyMsg("enter"))
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("the prompt should close, got phase %d", m.phase)
	}
	if cmd == nil {
		t.Fatal("no work was started")
	}
	if !m.busy {
		t.Fatal("the picker should be busy while git works")
	}
}

func TestEmptyNameIsRejected(t *testing.T) {
	m := testModel("master")
	m.phase = phaseName
	next, cmd := m.handleNameKey(keyMsg("enter"))
	m = next.(model)

	if cmd != nil {
		t.Fatal("an empty name should not start any work")
	}
	if m.nameErr == "" {
		t.Fatal("an empty name should show an error")
	}
	if m.phase != phaseName {
		t.Fatalf("the prompt should stay open, got phase %d", m.phase)
	}
}

func TestInvalidNameIsRejected(t *testing.T) {
	m := testModel("master")
	m.phase = phaseName
	m.nameBuf = "has space"
	next, cmd := m.handleNameKey(keyMsg("enter"))
	m = next.(model)

	if cmd != nil {
		t.Fatal("an invalid name should not start any work")
	}
	if m.nameErr == "" {
		t.Fatal("an invalid name should show an error")
	}
}

func TestEscapeInTheNamePromptGoesBack(t *testing.T) {
	m := testModel("master")
	m.phase = phaseName
	next, _ := m.handleNameKey(keyMsg("esc"))
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("escape should return to the list, got phase %d", m.phase)
	}
}

func TestEscapeLeavesThePicker(t *testing.T) {
	m := testModel("master")
	next, cmd := m.handleKey(keyMsg("esc"))
	_ = next

	if cmd == nil {
		t.Fatal("escape should quit the picker")
	}
}

func TestCtrlCStartsACommit(t *testing.T) {
	// Ctrl+C is the commit key now, so it must not leave the picker.
	m := testModel("master")
	next, cmd := m.handleKey(keyMsg("ctrl+c"))
	m = next.(model)

	if cmd == nil {
		t.Fatal("ctrl+c should have started looking for changes")
	}
	if m.phase != phaseFiles {
		t.Fatalf("ctrl+c should open the file selection, got phase %d", m.phase)
	}
}

func TestCtrlPStartsAPush(t *testing.T) {
	m := testModel("master")
	next, cmd := m.handleKey(keyMsg("ctrl+p"))
	m = next.(model)

	if cmd == nil {
		t.Fatal("ctrl+p should have started the push")
	}
	if !m.busy {
		t.Fatal("the picker should be busy while git works")
	}
}

func TestKeysAreHeldBackWhileGitIsWorking(t *testing.T) {
	m := testModel("master", "feature/x")
	m.busy = true
	m.cursor = 0

	// A switch that arrives while git is working waits its turn, because
	// git will not let two commands touch the index at once.
	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("a second switch should not have been started")
	}
	if m.pending == nil {
		t.Fatal("the switch should have been held back")
	}
	if m.pending.branch.Name != "master" {
		t.Fatalf("the wrong branch was held back: %q", m.pending.branch.Name)
	}
}

func TestEscapeLeavesEvenWhileGitIsWorking(t *testing.T) {
	m := testModel("master")
	m.busy = true
	m.pulling = true

	next, cmd := m.handleKey(keyMsg("esc"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("escape should quit the picker even while git is working")
	}
}

func TestControlByteOpensTheNewBranchPrompt(t *testing.T) {
	// A console in virtual terminal input mode hands over the raw control
	// byte with no modifier flag, so ctrl+n arrives as an ordinary rune.
	m := testModel("master", "feature/x")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x0e}})
	m = next.(model)

	if m.phase != phaseName {
		t.Fatalf("ctrl+n should open the name prompt, got phase %d", m.phase)
	}
}

func TestControlByteTogglesTheSort(t *testing.T) {
	m := testModel("master", "feature/x")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x13}})
	m = next.(model)

	if m.sortMode != sortName {
		t.Fatalf("ctrl+s should toggle the sort, got %q", m.sortMode)
	}
}

func TestControlByteNeverReachesTheSearchBox(t *testing.T) {
	// A stray control byte in the search box filters the list away, which
	// leaves nothing to select and makes the app look broken.
	m := testModel("master", "feature/x")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x0e}})
	m = next.(model)

	if m.query != "" {
		t.Fatalf("the search box should be empty, got %q", m.query)
	}
	if len(m.matches) != 2 {
		t.Fatalf("the list should be whole, got %d rows", len(m.matches))
	}
}

func TestControlKeyName(t *testing.T) {
	cases := []struct {
		r    rune
		want string
	}{
		{0x01, "ctrl+a"},
		{0x0e, "ctrl+n"},
		{0x13, "ctrl+s"},
		{0x1a, "ctrl+z"},
		{0x09, "tab"},
		{0x0d, "enter"},
		{0x1b, "esc"},
		{0x7f, "backspace"},
	}
	for _, c := range cases {
		got, ok := controlKeyName(c.r)
		if !ok || got != c.want {
			t.Errorf("controlKeyName(%#x): got %q (%v), want %q", c.r, got, ok, c.want)
		}
	}
	if _, ok := controlKeyName('a'); ok {
		t.Error("a printable character is not a control key")
	}
}

func TestStripControl(t *testing.T) {
	if got := stripControl("ma\x0ein"); got != "main" {
		t.Fatalf("stripControl: got %q, want %q", got, "main")
	}
}

func TestSwitchingKeepsThePickerOpen(t *testing.T) {
	m := testModel("master", "feature/x")
	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(model)

	if cmd == nil {
		t.Fatal("enter should start the switch")
	}
	if m.busy != true {
		t.Fatal("the picker should be busy while git works")
	}
}
