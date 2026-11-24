package main

// Tests for the help panel, the output toggle and the hint that wraps.

import (
	"strings"
	"testing"
)

func TestTheHeaderHasNoPointerOnIt(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	head := strings.SplitN(visible(m.View()), "\n", 2)[0]
	if strings.Contains(head, pointer) {
		t.Fatalf("the header should not have the pointer on it: %q", head)
	}
	if strings.Contains(head, "?") {
		t.Fatalf("the header should not have a question mark in it: %q", head)
	}
}

func TestThePointerIsOnlyOnTheRowThatIsPicked(t *testing.T) {
	m := testModel("master", "feature/x")
	m.width = 80
	m.height = 20

	lines := strings.Split(visible(m.View()), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, pointer) {
			continue
		}
		// The pointer is on the branch that is picked, which is the
		// only line that has the name of a branch on it.
		if !strings.Contains(line, "master") && !strings.Contains(line, "feature/x") {
			t.Fatalf("the pointer is on a line that is not a branch: %q", line)
		}
		if i == 0 {
			t.Fatal("the pointer should not be on the header")
		}
	}
}

func TestTheHintWrapsOntoASecondLine(t *testing.T) {
	m := testModel("master", "feature/x")
	m.width = 80
	m.height = 20

	lines := instructionLines(m.width)
	if len(lines) != 2 {
		t.Fatalf("the hint should be on two lines at 80 wide: got %d", len(lines))
	}
	joined := strings.Join(lines, " ")
	for _, want := range []string{"ctrl+o output", "ctrl+u update", "? help", "esc exits"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the hint is missing %q: %q", want, joined)
		}
	}
}
func TestTheHintKeepsTheWholeTextUntilItHasToChange(t *testing.T) {
	wide := instructionLines(200)
	if len(wide) != len(instruction) {
		t.Fatalf("the hint should be the whole text: got %d lines, want %d", len(wide), len(instruction))
	}
	for i, line := range wide {
		if line != instruction[i] {
			t.Fatalf("the hint should be the whole text: got %q, want %q", line, instruction[i])
		}
	}

	// A narrow window gets the shorter forms, but only when the whole text
	// will not fit.
	narrow := instructionLines(40)
	if len(narrow) != 1 {
		t.Fatalf("a narrow window should get a one line hint: got %d", len(narrow))
	}
	if narrow[0] == instruction[0] {
		t.Fatal("a narrow window should get a shorter hint")
	}
}

func TestTheViewStillFillsTheScreenWhenTheHintWraps(t *testing.T) {
	for _, width := range []int{60, 80, 100, 120} {
		m := testModel("master", "feature/x", "release/1")
		m.width = width
		m.height = 20

		lines := strings.Split(m.View(), "\n")
		if len(lines) != m.height {
			t.Fatalf("the view should fill the screen at %d wide: got %d rows, want %d", width, len(lines), m.height)
		}
	}
}

func TestTheHintIsOnOneLineWhenThereIsRoom(t *testing.T) {
	m := testModel("master")
	m.width = 200
	m.height = 20

	view := visible(m.View())
	for _, line := range instruction {
		if !strings.Contains(view, line) {
			t.Fatalf("the whole hint should be on the screen: %q", view)
		}
	}
}

func TestTheQuestionMarkOpensTheHelp(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	next, _ := m.handleKey(keyMsg("?"))
	m = next.(model)
	if m.phase != phaseHelp {
		t.Fatalf("the question mark should open the help: phase %d", m.phase)
	}

	view := visible(m.View())
	for _, want := range []string{"ctrl+o", "ctrl+u", "esc"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the help is missing %q: %q", want, view)
		}
	}
}

func TestTheHelpClosesOnEscape(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.phase = phaseHelp

	next, _ := m.handleKey(keyMsg("esc"))
	m = next.(model)
	if m.phase != phaseList {
		t.Fatalf("escape should leave the help: phase %d", m.phase)
	}
}

func TestTheQuestionMarkStillFiltersInTheSearch(t *testing.T) {
	// The question mark is only a key in the branch list. In the commit
	// message it is ordinary text.
	m := testModel("master")
	m.phase = phaseMessage
	m.messageBuf = ""
	m.messagePos = 0

	next, _ := m.handleKey(keyMsg("?"))
	m = next.(model)
	if m.messageBuf != "?" {
		t.Fatalf("the question mark should be typed into the message: %q", m.messageBuf)
	}
}

func TestCtrlOShowsWhatGitLastSaid(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.addOutput("git switch master", "Switched to branch 'master'")

	// The output is closed, and the text is kept.
	m.output = ""
	m.outputTitle = ""

	next, _ := m.handleKey(keyMsg("ctrl+o"))
	m = next.(model)
	if m.phase != phaseOutput {
		t.Fatalf("ctrl+o should open the output: phase %d", m.phase)
	}
	if !strings.Contains(visible(m.View()), "Switched to branch") {
		t.Fatalf("the output should be on screen: %q", visible(m.View()))
	}
}

func TestCtrlOHidesTheOutputAgain(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.addOutput("git switch master", "Switched to branch 'master'")
	m.showOutput()

	next, _ := m.handleKey(keyMsg("ctrl+o"))
	m = next.(model)
	if m.phase != phaseList {
		t.Fatalf("ctrl+o should close the output: phase %d", m.phase)
	}
}

func TestCtrlOSaysWhenThereIsNothingToShow(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	next, _ := m.handleKey(keyMsg("ctrl+o"))
	m = next.(model)
	if m.phase != phaseList {
		t.Fatalf("the output should stay away: phase %d", m.phase)
	}
	if !strings.Contains(m.status, "nothing from git") {
		t.Fatalf("the status should say there is nothing: %q", m.status)
	}
}

func TestTheOutputIsKeptAfterItIsClosed(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.addOutput("git push", "Everything up to date")
	m.showOutput()

	next, _ := m.handleKey(keyMsg("esc"))
	m = next.(model)
	if m.lastOutput == "" {
		t.Fatal("the output should be kept after the panel is closed")
	}
}

func TestCtrlUAsksForTheNewestBuild(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	next, cmd := m.handleKey(keyMsg("ctrl+u"))
	m = next.(model)
	if !m.updating {
		t.Fatal("ctrl+u should start the update")
	}
	if cmd == nil {
		t.Fatal("ctrl+u should run a command")
	}
}

func TestTheUpdateSaysWhenThereIsNothingNewer(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20

	// The published build is the one that is running, so there is nothing
	// newer. The version of the program is used, so that the test does not
	// have to be changed every time the version is.
	next, _ := m.Update(updateDoneMsg{version: version})
	m = next.(model)
	if m.updating {
		t.Fatal("the update should be finished")
	}
	if m.phase == phaseUpdateConfirm {
		t.Fatal("there should be no question when nothing was replaced")
	}
	if !strings.Contains(m.status, "newest build") {
		t.Fatalf("the status should say the build is the newest: %q", m.status)
	}
}

func TestTheUpdateAsksWhetherToStartTheNewBuild(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.updating = true

	next, _ := m.Update(updateDoneMsg{version: "v1.11", updated: true})
	m = next.(model)
	if m.phase != phaseUpdateConfirm {
		t.Fatalf("the picker should ask about the new build: phase %d", m.phase)
	}

	view := visible(m.View())
	if !strings.Contains(view, "v1.11") {
		t.Fatalf("the question should say what was installed: %q", view)
	}
}

func TestSayingNoKeepsThePickerOpen(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.phase = phaseUpdateConfirm
	m.updatedTo = "v1.11"

	next, _ := m.handleKey(keyMsg("n"))
	m = next.(model)
	if m.phase != phaseList {
		t.Fatalf("the picker should stay open: phase %d", m.phase)
	}
	if !strings.Contains(m.status, "v1.11") {
		t.Fatalf("the status should say what happened: %q", m.status)
	}
}

func TestSayingYesHandsTheConsoleOver(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.phase = phaseUpdateConfirm
	m.updatedTo = "v1.11"

	next, cmd := m.handleKey(keyMsg("y"))
	m = next.(model)
	if m.relaunch == "" {
		t.Fatal("the new build should be named")
	}
	if cmd == nil {
		t.Fatal("the picker should quit")
	}
}

func TestTheFailedUpdateSaysWhatWentWrong(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.updating = true

	next, _ := m.Update(updateDoneMsg{err: errTest("the network is down")})
	m = next.(model)
	if m.phase == phaseUpdateConfirm {
		t.Fatal("a failed update should not ask about a new build")
	}
	if !strings.Contains(m.status, "the network is down") {
		t.Fatalf("the status should say what went wrong: %q", m.status)
	}
}

// errTest is an error with a message.
type errTest string

func (e errTest) Error() string { return string(e) }

// theHelpScrolls keeps the pointer of the help inside the panel.
func TestTheHelpScrollsAndStopsAtTheEnd(t *testing.T) {
	m := testModel("master")
	m.width = 80
	m.height = 20
	m.phase = phaseHelp

	for i := 0; i < 50; i++ {
		next, _ := m.handleKey(keyMsg("down"))
		m = next.(model)
	}
	if m.helpScroll <= 0 {
		t.Fatal("the help should scroll")
	}
	if m.helpScroll > len(helpLines()) {
		t.Fatalf("the help scrolled past the end: %d of %d", m.helpScroll, len(helpLines()))
	}
}
