package main

// Tests for cloning: finding an address, the folder it goes into, the
// question about a folder that is in the way, and the picker moving into
// the repository that was cloned.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// runesMsg is a key press that carries a whole piece of text.
func runesMsg(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func TestAGitAddressIsFoundInText(t *testing.T) {
	addresses := []string{
		"git@github.com:owner/repo.git",
		"git@github.com:owner/repo",
		"ssh://git@github.com/owner/repo.git",
		"https://github.com/owner/repo.git",
		"https://github.com/owner/repo",
		"http://gitlab.com/owner/repo.git",
		"https://bitbucket.org/owner/repo",
		"https://codeberg.org/owner/repo",
	}
	for _, address := range addresses {
		if got := gitAddress(address); got != address {
			t.Errorf("gitAddress(%q): got %q", address, got)
		}
	}
}

func TestAGitAddressIsFoundInTheMiddleOfSomeText(t *testing.T) {
	text := "here is the repository\nhttps://github.com/owner/repo.git\nthanks"

	got := gitAddress(text)

	if got != "https://github.com/owner/repo.git" {
		t.Fatalf("the address: got %q", got)
	}
}

func TestOrdinaryTextIsNotAGitAddress(t *testing.T) {
	texts := []string{
		"",
		"just some text I copied",
		"https://example.com/some/page",
		"https://github.com",
		"mailto:someone@example.com",
		"git@github.com",
	}
	for _, text := range texts {
		if got := gitAddress(text); got != "" {
			t.Errorf("gitAddress(%q): got %q, want nothing", text, got)
		}
	}
}

func TestTheFolderNameComesFromTheAddress(t *testing.T) {
	cases := map[string]string{
		"https://github.com/owner/repo.git":    "repo",
		"https://github.com/owner/repo":        "repo",
		"git@github.com:owner/repo.git":        "repo",
		"https://github.com/owner/repo/":       "repo",
		"https://github.com/owner/my-repo.git": "my-repo",
	}
	for address, want := range cases {
		if got := repoFolder(address); got != want {
			t.Errorf("repoFolder(%q): got %q, want %q", address, got, want)
		}
	}
}

func TestTheCloneGoesIntoTheCurrentDirectory(t *testing.T) {
	dir := filepath.Join("C:", "work")

	target, ok := cloneTarget("https://github.com/owner/repo.git", dir)

	if !ok {
		t.Fatal("the target should have been worked out")
	}
	if target != filepath.Join(dir, "repo") {
		t.Fatalf("the target: got %q", target)
	}
}

func TestTheCloneRunsAndThePickerMovesIntoIt(t *testing.T) {
	_, remote := makeTestRepoWithRemote(t)
	target := t.TempDir()

	m := testModel("master")
	m.cloneDir = target
	m.phase = phaseClone

	// The address is typed and confirmed.
	next, _ := m.handleKey(runesMsg(remote))
	m = next.(model)
	if m.cloneBuf != remote {
		t.Fatalf("the address: got %q", m.cloneBuf)
	}

	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("enter should have started the clone")
	}

	done, ok := cmd().(cloneDoneMsg)
	if !ok {
		t.Fatal("the clone should have reported back")
	}
	if done.err != nil {
		t.Fatalf("the clone failed: %v", done.err)
	}

	// The folder is there, with the branches in it.
	folder := filepath.Join(target, repoFolder(remote))
	if !pathExists(folder) {
		t.Fatalf("the clone should have made %q", folder)
	}
	branches, err := listBranches(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) == 0 {
		t.Fatal("the clone should have the branches in it")
	}

	// And the picker moves into it.
	next, _ = m.Update(done)
	m = next.(model)
	if m.root != folder {
		t.Fatalf("the picker should be in %q, got %q", folder, m.root)
	}
	if m.current == "" {
		t.Fatal("the picker should know the branch it is on")
	}
}

func TestAnExistingFolderIsOnlyRemovedAfterTheQuestion(t *testing.T) {
	_, remote := makeTestRepoWithRemote(t)
	target := t.TempDir()

	folder := filepath.Join(target, repoFolder(remote))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(folder, "marker.txt")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := testModel("master")
	m.cloneDir = target
	m.phase = phaseClone

	next, _ := m.handleKey(runesMsg(remote))
	m = next.(model)

	// Enter asks about the folder that is in the way, and touches
	// nothing.
	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(model)
	if m.phase != phaseRemove {
		t.Fatalf("the picker should ask about the folder, got phase %d", m.phase)
	}
	if cmd != nil {
		t.Fatal("nothing should have started yet")
	}
	if !pathExists(marker) {
		t.Fatal("the folder should not have been touched yet")
	}

	// Saying no leaves it alone and goes back to the address.
	next, cmd = m.handleKey(keyMsg("n"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("no should not have started anything")
	}
	if !pathExists(marker) {
		t.Fatal("the folder should still be there")
	}
	if m.phase != phaseClone {
		t.Fatalf("the picker should be back at the address, got phase %d", m.phase)
	}
	if m.cloneBuf != remote {
		t.Fatalf("the address should still be there, got %q", m.cloneBuf)
	}

	// Confirming it again asks about the folder once more, and saying
	// yes removes it and clones again.
	next, _ = m.handleKey(keyMsg("enter"))
	m = next.(model)
	if m.phase != phaseRemove {
		t.Fatalf("the picker should ask about the folder, got phase %d", m.phase)
	}
	next, cmd = m.handleKey(keyMsg("y"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("yes should have started the clone")
	}
	done, ok := cmd().(cloneDoneMsg)
	if !ok {
		t.Fatal("the clone should have reported back")
	}
	if done.err != nil {
		t.Fatalf("the clone failed: %v", done.err)
	}
	if pathExists(marker) {
		t.Fatal("the folder should have been removed")
	}
}

func TestTheCloneOnTheClipboardIsOfferedWhenThereIsNoRepository(t *testing.T) {
	m := testModel("master")
	m.root = ""
	m.repoName = "folder"

	next, _ := m.Update(clipboardMsg{address: "https://github.com/owner/repo.git"})
	m = next.(model)

	if m.phase != phaseClipboard {
		t.Fatalf("the clone should be offered, got phase %d", m.phase)
	}
	if m.clipAddress != "https://github.com/owner/repo.git" {
		t.Fatalf("the address: got %q", m.clipAddress)
	}
}

func TestTheClipboardIsIgnoredInsideARepository(t *testing.T) {
	// Inside a repository there is nothing to clone, so the clipboard is
	// not looked at.
	m := testModel("master")

	next, _ := m.Update(clipboardMsg{address: "https://github.com/owner/repo.git"})
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("there should be no offer inside a repository, got phase %d", m.phase)
	}
}

func TestOrdinaryTextOnTheClipboardIsNotOffered(t *testing.T) {
	m := testModel("master")
	m.root = ""

	next, _ := m.Update(clipboardMsg{address: "the recipe for lasagne"})
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("ordinary text is not a repository, got phase %d", m.phase)
	}
}

func TestLeavingWithoutARepositoryIsAFailure(t *testing.T) {
	// The picker starts outside a repository now, but leaving it there is
	// still a failure, because there was nothing to work on.
	var out bytes.Buffer

	code := reportError(noRepositoryError(), &out)

	if code != 1 {
		t.Fatalf("the exit code: got %d, want 1", code)
	}
	if !strings.Contains(out.String(), "not inside a git repository") {
		t.Fatalf("the error should be reported:\n%s", out.String())
	}
}

func TestTheOutputPanelShowsWhatGitSaid(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 20
	m.phase = phaseOutput
	m.outputTitle = "git push"
	m.output = "stdout:\nFrom github.com:owner/repo\n  * [new branch]  feature/x -> feature/x"

	panel := visible(m.viewOutput())

	if !strings.Contains(panel, "git push") {
		t.Fatalf("the command should be named:\n%s", panel)
	}
	if !strings.Contains(panel, "[new branch]") {
		t.Fatalf("the output should be on screen:\n%s", panel)
	}
	if !strings.Contains(panel, "esc closes the output") {
		t.Fatalf("the panel should say how to close it:\n%s", panel)
	}
}

func TestTheOutputScrolls(t *testing.T) {
	m := testModel("master")
	m.width = 70
	m.height = 10
	m.phase = phaseOutput
	m.output = strings.Repeat("a line of output\n", 40)

	// The first page is what is on screen to start with.
	if m.outputScroll != 0 {
		t.Fatalf("the output should start at the top, got %d", m.outputScroll)
	}

	next, _ := m.handleKey(keyMsg("down"))
	m = next.(model)
	if m.outputScroll != 1 {
		t.Fatalf("down should have moved a line, got %d", m.outputScroll)
	}

	next, _ = m.handleKey(keyMsg("pgdn"))
	m = next.(model)
	if m.outputScroll != 1+m.outputRows() {
		t.Fatalf("a page down should have moved a screenful, got %d", m.outputScroll)
	}

	// The top cannot be scrolled past.
	for i := 0; i < 100; i++ {
		next, _ = m.handleKey(keyMsg("up"))
		m = next.(model)
	}
	if m.outputScroll != 0 {
		t.Fatalf("the output should not scroll past the top, got %d", m.outputScroll)
	}
}

func TestClosingTheOutputGoesBackToTheList(t *testing.T) {
	m := testModel("master")
	m.phase = phaseOutput
	m.output = "stdout:\nsomething"

	next, _ := m.handleKey(keyMsg("esc"))
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("the picker should be back on the list, got phase %d", m.phase)
	}
	if m.output != "" {
		t.Fatal("the output should be cleared")
	}
}
