package main

// Tests for the commit flow: the file selection, the commit message with
// its history, and the git work behind them.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// runGitIn runs a git command in a directory and returns its output.
func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// withHome points the history file at a temporary folder for the duration
// of a test.
func withHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := homeDir
	homeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { homeDir = old })
	return dir
}

func TestHistoryRoundTrip(t *testing.T) {
	dir := withHome(t)

	if got := loadHistory(); len(got) != 0 {
		t.Fatalf("expected no history, got %v", got)
	}

	messages := rememberMessage(nil, "first message")
	messages = rememberMessage(messages, "second message")
	if err := saveHistory(messages); err != nil {
		t.Fatal(err)
	}

	loaded := loadHistory()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %v", loaded)
	}
	if loaded[0] != "second message" || loaded[1] != "first message" {
		t.Fatalf("the newest message should be first, got %v", loaded)
	}

	// The file is a dot file in the home folder.
	if _, err := os.Stat(filepath.Join(dir, historyFileName)); err != nil {
		t.Fatalf("the history file was not written: %v", err)
	}
}

func TestHistoryKeepsTheLastTen(t *testing.T) {
	withHome(t)

	messages := []string{}
	for i := 0; i < 15; i++ {
		messages = rememberMessage(messages, "message "+string(rune('a'+i)))
	}
	if len(messages) != historyLimit {
		t.Fatalf("expected %d messages, got %d", historyLimit, len(messages))
	}
	if messages[0] != "message o" {
		t.Fatalf("the newest message should be first, got %q", messages[0])
	}
}

func TestHistoryDoesNotRepeatAMessage(t *testing.T) {
	withHome(t)

	messages := rememberMessage(nil, "same message")
	messages = rememberMessage(messages, "another")
	messages = rememberMessage(messages, "same message")

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", messages)
	}
	if messages[0] != "same message" {
		t.Fatalf("the repeated message should be at the front, got %q", messages[0])
	}
}

func TestHistorySurvivesARubbishFile(t *testing.T) {
	dir := withHome(t)

	if err := os.WriteFile(filepath.Join(dir, historyFileName), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadHistory(); len(got) != 0 {
		t.Fatalf("a rubbish file should give no history, got %v", got)
	}
}

func TestSpaceTogglesAFile(t *testing.T) {
	m := testModel("master")
	m.phase = phaseFiles
	m.files = []FileStatus{{Code: " M", Path: "a.go"}, {Code: "??", Path: "b.go"}}
	m.fileSelected = map[string]bool{}

	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	next, _ := m.handleKey(space)
	m = next.(model)
	if !m.fileSelected["a.go"] {
		t.Fatal("space should have selected the file")
	}

	// Space again takes the selection back off.
	next, _ = m.handleKey(space)
	m = next.(model)
	if m.fileSelected["a.go"] {
		t.Fatal("space should have taken the selection off")
	}

	// Moving down and selecting picks the other file.
	next, _ = m.handleKey(keyMsg("down"))
	m = next.(model)
	next, _ = m.handleKey(space)
	m = next.(model)
	if !m.fileSelected["b.go"] {
		t.Fatalf("the second file should be selected, got %v", m.fileSelected)
	}
}

func TestSelectedPaths(t *testing.T) {
	m := testModel("master")
	m.phase = phaseFiles
	m.files = []FileStatus{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}
	m.fileSelected = map[string]bool{"a.go": true, "c.go": true}

	paths := m.selectedPaths()
	if len(paths) != 2 || paths[0] != "a.go" || paths[1] != "c.go" {
		t.Fatalf("selected paths: got %v", paths)
	}
}

func TestEnterWithNothingSelectedIsRefused(t *testing.T) {
	m := testModel("master")
	m.phase = phaseFiles
	m.files = []FileStatus{{Path: "a.go"}}
	m.fileSelected = map[string]bool{}

	next, _ := m.handleKey(keyMsg("enter"))
	m = next.(model)
	if m.phase != phaseFiles {
		t.Fatalf("the message box should not open, got phase %d", m.phase)
	}
	if m.status == "" {
		t.Fatal("there should be a message about selecting a file")
	}
}

func TestEnterWithASelectionOpensTheMessageBox(t *testing.T) {
	m := testModel("master")
	m.phase = phaseFiles
	m.files = []FileStatus{{Path: "a.go"}}
	m.fileSelected = map[string]bool{"a.go": true}

	next, _ := m.handleKey(keyMsg("enter"))
	m = next.(model)
	if m.phase != phaseMessage {
		t.Fatalf("the message box should open, got phase %d", m.phase)
	}
}

func TestHistoryRecallWalksBackAndForth(t *testing.T) {
	m := testModel("master")
	m.phase = phaseMessage
	m.history = []string{"newest", "older", "oldest"}
	m.historyIndex = -1

	next, _ := m.handleKey(keyMsg("ctrl+p"))
	m = next.(model)
	if m.messageBuf != "newest" {
		t.Fatalf("ctrl+p should recall the newest message, got %q", m.messageBuf)
	}

	next, _ = m.handleKey(keyMsg("ctrl+p"))
	m = next.(model)
	if m.messageBuf != "older" {
		t.Fatalf("ctrl+p should move to the older message, got %q", m.messageBuf)
	}

	next, _ = m.handleKey(keyMsg("ctrl+p"))
	m = next.(model)
	if m.messageBuf != "oldest" {
		t.Fatalf("ctrl+p should reach the oldest message, got %q", m.messageBuf)
	}

	next, _ = m.handleKey(keyMsg("ctrl+n"))
	m = next.(model)
	if m.messageBuf != "older" {
		t.Fatalf("ctrl+n should move back, got %q", m.messageBuf)
	}

	next, _ = m.handleKey(keyMsg("ctrl+n"))
	m = next.(model)
	if m.messageBuf != "newest" {
		t.Fatalf("ctrl+n should reach the newest message, got %q", m.messageBuf)
	}

	// And all the way down to an empty message.
	next, _ = m.handleKey(keyMsg("ctrl+n"))
	m = next.(model)
	if m.messageBuf != "" {
		t.Fatalf("ctrl+n should clear the message, got %q", m.messageBuf)
	}
}

func TestTypingLeavesTheHistory(t *testing.T) {
	m := testModel("master")
	m.phase = phaseMessage
	m.history = []string{"newest"}
	m.historyIndex = -1

	next, _ := m.handleKey(keyMsg("ctrl+p"))
	m = next.(model)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(model)

	if m.historyIndex != -1 {
		t.Fatalf("typing should leave the history, got index %d", m.historyIndex)
	}
	if m.messageBuf != "newestx" {
		t.Fatalf("the typed text should follow the recalled message, got %q", m.messageBuf)
	}
}

func TestTheMessageBoxRemembersWhatWasTyped(t *testing.T) {
	withHome(t)

	m := testModel("master")
	m.phase = phaseMessage
	m.history = []string{}
	m.historyIndex = -1
	m.files = []FileStatus{{Path: "a.go"}}
	m.fileSelected = map[string]bool{"a.go": true}

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a message")})
	m = next.(model)
	next, _ = m.handleKey(keyMsg("ctrl+enter"))
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("the commit should leave the message box, got phase %d", m.phase)
	}
	if len(m.history) != 1 || m.history[0] != "a message" {
		t.Fatalf("the message should have been remembered, got %v", m.history)
	}
	loaded := loadHistory()
	if len(loaded) != 1 || loaded[0] != "a message" {
		t.Fatalf("the message should have been saved, got %v", loaded)
	}
}

func TestSpaceIsTypedIntoTheMessage(t *testing.T) {
	// Bubbletea turns a single space into its own key type rather than a
	// rune, so it has to be picked out separately or every space in a
	// commit message would be dropped.
	m := testModel("master")
	m.phase = phaseMessage

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(model)
	if m.messageBuf != " " {
		t.Fatalf("the space should have been typed, got %q", m.messageBuf)
	}

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a message")})
	m = next.(model)
	if m.messageBuf != " a message" {
		t.Fatalf("the message should have kept its space, got %q", m.messageBuf)
	}
}

func TestEnterAddsALineToTheMessage(t *testing.T) {
	m := testModel("master")
	m.phase = phaseMessage

	next, _ := m.handleKey(keyMsg("enter"))
	m = next.(model)

	if m.phase != phaseMessage {
		t.Fatalf("enter should not leave the message box, got phase %d", m.phase)
	}
	if m.messageBuf != "\n" {
		t.Fatalf("enter should have added a line, got %q", m.messageBuf)
	}
}

func TestCtrlEnterCommitsAMessageOfMoreThanOneLine(t *testing.T) {
	withHome(t)

	m := testModel("master")
	m.phase = phaseMessage
	m.history = []string{}
	m.historyIndex = -1
	m.files = []FileStatus{{Path: "a.go"}}
	m.fileSelected = map[string]bool{"a.go": true}

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first")})
	m = next.(model)
	next, _ = m.handleKey(keyMsg("enter"))
	m = next.(model)
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second")})
	m = next.(model)

	if m.messageBuf != "first\nsecond" {
		t.Fatalf("the message: got %q", m.messageBuf)
	}

	next, cmd := m.handleKey(keyMsg("ctrl+enter"))
	m = next.(model)
	if m.phase != phaseList {
		t.Fatalf("ctrl+enter should leave the message box, got phase %d", m.phase)
	}
	if cmd == nil {
		t.Fatal("ctrl+enter should have started the commit")
	}

	// The message that is committed is the whole thing, lines and all.
	if len(m.history) != 1 || m.history[0] != "first\nsecond" {
		t.Fatalf("the message should have been remembered whole, got %v", m.history)
	}
}

func TestUpAndDownMoveTheCaretBetweenTheLines(t *testing.T) {
	m := testModel("master")
	m.phase = phaseMessage
	m.messageBuf = "first\nsecond"
	m.messagePos = len(m.messageBuf)

	next, _ := m.handleKey(keyMsg("up"))
	m = next.(model)
	if m.messagePos != 5 {
		t.Fatalf("up should move the caret to the end of the first line, got %d", m.messagePos)
	}

	next, _ = m.handleKey(keyMsg("down"))
	m = next.(model)
	if m.messagePos != 11 {
		t.Fatalf("down should move the caret back, got %d", m.messagePos)
	}
}

func TestTheMessageBoxShowsEveryLine(t *testing.T) {
	m := testModel("master")
	m.phase = phaseMessage
	m.messageBuf = "first\nsecond"
	m.messagePos = len(m.messageBuf)
	m.height = 24

	view := visible(m.View())
	if !strings.Contains(view, "first") || !strings.Contains(view, "second") {
		t.Fatalf("the box should show both lines:\n%s", view)
	}
}

func TestASuccessfulCommitAsksAboutThePush(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(commitDoneMsg{message: "a message", files: 1})
	m = next.(model)

	// What git said about the commit comes first.
	if m.phase != phaseOutput {
		t.Fatalf("the output should be on screen, got phase %d", m.phase)
	}
	if m.afterOutput != afterPushConfirm {
		t.Fatalf("the push should be offered after the output, got %q", m.afterOutput)
	}

	// Closing the output leads to the question.
	next, _ = m.handleKey(keyMsg("esc"))
	m = next.(model)

	if m.phase != phasePushConfirm {
		t.Fatalf("the push should be offered, got phase %d", m.phase)
	}
	if m.pushChoice != 0 {
		t.Fatal("yes should be the starting answer")
	}
}

func TestAFailedCommitDoesNotAskAboutThePush(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(commitDoneMsg{message: "a message", err: os.ErrInvalid})
	m = next.(model)

	if m.phase == phasePushConfirm {
		t.Fatal("a failed commit should not offer a push")
	}
	// A failed commit is one of the failures that ends the picker.
	if m.phase != phaseError {
		t.Fatalf("a failed commit should end the picker, got phase %d", m.phase)
	}
}

func TestAnsweringYesPushes(t *testing.T) {
	m := testModel("master")
	m.phase = phasePushConfirm

	next, cmd := m.handleKey(keyMsg("y"))
	m = next.(model)

	if cmd == nil {
		t.Fatal("yes should have started the push")
	}
	if m.phase != phaseList {
		t.Fatalf("the picker should be back on the list, got phase %d", m.phase)
	}
}

func TestAnsweringNoDoesNotPush(t *testing.T) {
	m := testModel("master")
	m.phase = phasePushConfirm

	next, cmd := m.handleKey(keyMsg("n"))
	m = next.(model)

	if cmd != nil {
		t.Fatal("no should not have started anything")
	}
	if m.phase != phaseList {
		t.Fatalf("the picker should be back on the list, got phase %d", m.phase)
	}
}

func TestThePushQuestionCanBeAnsweredWithTheArrowKeys(t *testing.T) {
	m := testModel("master")
	m.phase = phasePushConfirm

	// Down moves to no, and enter takes it.
	next, _ := m.handleKey(keyMsg("down"))
	m = next.(model)
	if m.pushChoice != 1 {
		t.Fatalf("down should have moved to no, got %d", m.pushChoice)
	}
	next, cmd := m.handleKey(keyMsg("enter"))
	m = next.(model)
	if cmd != nil {
		t.Fatal("no should not have started anything")
	}
	if m.phase != phaseList {
		t.Fatalf("the picker should be back on the list, got phase %d", m.phase)
	}
}

func TestStatusFiles(t *testing.T) {
	repo := makeTestRepo(t)

	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitIn(t, repo, "add", "tracked.txt")
	runGitIn(t, repo, "commit", "-q", "-m", "add tracked")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, _, err := statusFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %v", files)
	}

	codes := map[string]string{}
	for _, file := range files {
		codes[file.Path] = file.Code
	}
	if codes["new.txt"] != "??" {
		t.Errorf("an untracked file should be ??, got %q", codes["new.txt"])
	}
	if !strings.Contains(codes["tracked.txt"], "M") {
		t.Errorf("a modified file should be marked M, got %q", codes["tracked.txt"])
	}
}

func TestCommitFilesCommitsOnlyWhatWasChosen(t *testing.T) {
	repo := makeTestRepo(t)

	chosen := filepath.Join(repo, "chosen.txt")
	other := filepath.Join(repo, "other.txt")
	if err := os.WriteFile(chosen, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := commitFiles(repo, "a commit message", []string{"chosen.txt"}); err != nil {
		t.Fatal(err)
	}

	subject := runGitIn(t, repo, "log", "-1", "--format=%s")
	if strings.TrimSpace(subject) != "a commit message" {
		t.Fatalf("commit message: got %q", strings.TrimSpace(subject))
	}

	// The chosen file is in the commit and the other one is not.
	files := runGitIn(t, repo, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "chosen.txt") {
		t.Fatalf("the chosen file should be in the commit, got %q", files)
	}
	if strings.Contains(files, "other.txt") {
		t.Fatalf("the other file should not be in the commit, got %q", files)
	}

	// And the other file is still waiting to be committed.
	remaining, _, err := statusFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Path != "other.txt" {
		t.Fatalf("the other file should still be there, got %v", remaining)
	}
}

func TestCommitFilesCommitsAMessageOfMoreThanOneLine(t *testing.T) {
	repo := makeTestRepo(t)

	chosen := filepath.Join(repo, "chosen.txt")
	if err := os.WriteFile(chosen, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := commitFiles(repo, "first line\nsecond line", []string{"chosen.txt"}); err != nil {
		t.Fatal(err)
	}

	// Git takes the message as it is, so both lines are in the commit.
	body := runGitIn(t, repo, "log", "-1", "--format=%B")
	if !strings.Contains(body, "first line") {
		t.Fatalf("the first line should be in the commit, got %q", body)
	}
	if !strings.Contains(body, "second line") {
		t.Fatalf("the second line should be in the commit, got %q", body)
	}
}

func TestCommitFilesRefusesAnEmptyMessage(t *testing.T) {
	repo := makeTestRepo(t)
	if _, err := commitFiles(repo, "  ", []string{"a.txt"}); err == nil {
		t.Fatal("an empty message should be refused")
	}
	if _, err := commitFiles(repo, "a message", nil); err == nil {
		t.Fatal("no files should be refused")
	}
}

func TestPush(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "remote.git")
	repo := filepath.Join(dir, "work")

	runGitIn(t, dir, "init", "-q", "--bare", bare)
	runGitIn(t, dir, "init", "-q", "-b", "main", repo)
	runGitIn(t, repo, "config", "user.email", "t@t.t")
	runGitIn(t, repo, "config", "user.name", "t")
	runGitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "first")
	runGitIn(t, repo, "remote", "add", "origin", bare)
	runGitIn(t, repo, "push", "-q", "-u", "origin", "main")

	// A second commit to push.
	runGitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "second")
	if _, err := push(repo); err != nil {
		t.Fatalf("the push failed: %v", err)
	}

	// The remote has the commit.
	remote := runGitIn(t, bare, "log", "-1", "--format=%s", "main")
	if strings.TrimSpace(remote) != "second" {
		t.Fatalf("the remote should have the commit, got %q", strings.TrimSpace(remote))
	}
}
