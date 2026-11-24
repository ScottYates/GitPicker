package main

// End to end tests that drive the real picker.
//
// The picker is run with a pipe for input and a buffer for output, so the
// whole program is exercised: the keys, the git work, the refresh of the
// list and the decision to stay open or to leave.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// runPicker starts the picker on a pipe and returns the buffer it draws to,
// the pipe to write keys into, and a channel that receives the final model.
func runPicker(t *testing.T, repo string) (*bytes.Buffer, *os.File, <-chan tea.Model, chan error) {
	t.Helper()

	branches, err := listBranches(repo)
	if err != nil {
		t.Fatal(err)
	}

	fetchCh := make(chan fetchResult, 1)
	fetchCh <- fetchResult{branches: branches}

	m := newModel(repo, "repo", currentBranch(repo), branches, fetchCh, nil)

	return startPicker(t, m)
}

// runPickerWithoutRepo starts the picker where there is no repository.
//
// It is the state the picker is in when it is started outside one, which
// is when there may be a clone to do.
func runPickerWithoutRepo(t *testing.T, dir string) (*bytes.Buffer, *os.File, <-chan tea.Model, chan error) {
	t.Helper()

	m := newModel("", filepath.Base(dir), "", nil, nil, nil)
	m.cloneDir = dir
	m.status = "not inside a git repository"
	m.statusErr = true

	return startPicker(t, m)
}

// startPicker runs a picker on a pipe and hands back the ends of it.
func startPicker(t *testing.T, m model) (*bytes.Buffer, *os.File, <-chan tea.Model, chan error) {
	t.Helper()

	inReader, inWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	program := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithInput(inReader),
		tea.WithOutput(&output),
	)

	results := make(chan tea.Model, 1)
	failures := make(chan error, 1)
	go func() {
		final, err := program.Run()
		if err != nil {
			failures <- err
			return
		}
		results <- final
	}()

	return &output, inWriter, results, failures
}

// waitFor polls until the condition holds or the deadline passes.
func waitFor(t *testing.T, what string, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
	return false
}

func TestPickerStaysOpenAfterSwitching(t *testing.T) {
	repo := makeTestRepo(t)
	output, input, results, failures := runPicker(t, repo)
	defer input.Close()

	// Let the first frame settle.
	time.Sleep(400 * time.Millisecond)

	// Press enter to switch to the highlighted branch. The first row is
	// the pinned branch, which is main.
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}

	// The output panel is the only screen that says this, so it cannot
	// be confused with the command preview.
	waitFor(t, "the switch to finish", func() bool {
		return strings.Contains(output.String(), "esc closes the output")
	})

	// Closing the output brings the picker back, and the switch is
	// reported in the status line.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the switch to be reported", func() bool {
		return strings.Contains(output.String(), "Switched to main")
	})

	// The picker must still be running: switching does not close it.
	select {
	case final := <-results:
		t.Fatalf("the picker closed itself after switching: %v", final)
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	default:
	}

	// And the repository really did move.
	if got := currentBranch(repo); got != "main" {
		t.Fatalf("the repo should be on main, got %q", got)
	}

	// Escape is the exit key.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("escape did not close the picker")
	}
}

func TestPickerCommitsTheSelectedFile(t *testing.T) {
	// The history file goes to a temporary folder for the test.
	home := withHome(t)

	repo := makeTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "chosen.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "other.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, input, results, failures := runPicker(t, repo)
	defer input.Close()

	time.Sleep(400 * time.Millisecond)

	// Ctrl+C opens the file selection.
	if _, err := input.Write([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file list", func() bool {
		return strings.Contains(output.String(), "Select files to commit")
	})
	waitFor(t, "the files to be listed", func() bool {
		return strings.Contains(output.String(), "chosen.txt")
	})

	// Space selects the first file, which is the one the pointer is on.
	if _, err := input.Write([]byte(" ")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file to be selected", func() bool {
		return strings.Contains(output.String(), "1 selected")
	})

	// Enter opens the message box.
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the message box", func() bool {
		return strings.Contains(output.String(), "Commit message")
	})

	// Type a message and confirm it.
	if _, err := input.Write([]byte("a commit message")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	// Ctrl+Enter finishes the message. A terminal sends it as the same
	// byte as ctrl+j, which is what a line feed is.
	if _, err := input.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}

	// The commit is done, so what git said about it is on screen.
	waitFor(t, "the commit output", func() bool {
		return strings.Contains(output.String(), "esc closes the output")
	})

	// The commit is really in the repository, and only the chosen file is
	// in it.
	subject := runGitIn(t, repo, "log", "-1", "--format=%s")
	if strings.TrimSpace(subject) != "a commit message" {
		t.Fatalf("the commit message: got %q", strings.TrimSpace(subject))
	}
	files := runGitIn(t, repo, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "chosen.txt") {
		t.Fatalf("the chosen file should be in the commit, got %q", files)
	}
	if strings.Contains(files, "other.txt") {
		t.Fatalf("the other file should not be in the commit, got %q", files)
	}

	// And the message was remembered.
	loaded := loadHistory()
	if len(loaded) == 0 || loaded[0] != "a commit message" {
		t.Fatalf("the message should have been remembered, got %v", loaded)
	}
	if _, err := os.Stat(filepath.Join(home, historyFileName)); err != nil {
		t.Fatalf("the history file was not written: %v", err)
	}

	// Closing the output leads to the question about pushing.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the push to be offered", func() bool {
		return strings.Contains(output.String(), "Push main?")
	})

	// Saying no to the push leaves the commit alone and goes back to the
	// list.
	if _, err := input.Write([]byte("n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the list to come back", func() bool {
		return strings.Contains(output.String(), "Switch to branch")
	})

	// Escape leaves.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("escape did not close the picker")
	}
}

func TestAnsweringYesPushesTheCommitToTheRemote(t *testing.T) {
	withHome(t)

	repo, remote := makeTestRepoWithRemote(t)
	if err := os.WriteFile(filepath.Join(repo, "pushed.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, input, results, failures := runPicker(t, repo)
	defer input.Close()

	time.Sleep(400 * time.Millisecond)

	// Ctrl+C opens the file selection.
	if _, err := input.Write([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file list", func() bool {
		return strings.Contains(output.String(), "Select files to commit")
	})
	waitFor(t, "the files to be listed", func() bool {
		return strings.Contains(output.String(), "pushed.txt")
	})

	// Space selects the file the pointer is on, enter opens the message.
	if _, err := input.Write([]byte(" ")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the file to be selected", func() bool {
		return strings.Contains(output.String(), "1 selected")
	})
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the message box", func() bool {
		return strings.Contains(output.String(), "Commit message")
	})

	if _, err := input.Write([]byte("a pushed commit")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	// Ctrl+Enter finishes the message.
	if _, err := input.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}

	// The commit output comes first, and closing it leads to the question
	// about pushing.
	waitFor(t, "the commit output", func() bool {
		return strings.Contains(output.String(), "esc closes the output")
	})
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}

	// The push is offered. The branch has an upstream here, so the
	// question names it.
	waitFor(t, "the push to be offered", func() bool {
		return strings.Contains(output.String(), "Push main to origin/main?")
	})

	// Saying yes pushes it.
	if _, err := input.Write([]byte("y")); err != nil {
		t.Fatal(err)
	}
	// The push panel is the one that names the push, and nothing else
	// on screen does.
	waitFor(t, "the push output", func() bool {
		return strings.Contains(output.String(), "git push")
	})

	// And the remote really has the commit, not just the local repository.
	subject := strings.TrimSpace(runGitIn(t, remote, "log", "-1", "--format=%s", "main"))
	if subject != "a pushed commit" {
		t.Fatalf("the remote should have the commit, got %q", subject)
	}
	files := runGitIn(t, remote, "show", "--name-only", "--format=", "main")
	if !strings.Contains(files, "pushed.txt") {
		t.Fatalf("the remote should have the file, got %q", files)
	}

	// The picker is still running.
	select {
	case final := <-results:
		t.Fatalf("the picker closed itself after pushing: %v", final)
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	default:
	}

	// The push output is still on screen, so the first escape closes it
	// and the second leaves.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the output to be closed", func() bool {
		return strings.Contains(output.String(), "Switch to branch")
	})

	// Escape leaves.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("escape did not close the picker")
	}
}

func TestThePickerEndsWithGitsExitCodeWhenAPullFails(t *testing.T) {
	withHome(t)

	repo, _ := makeTestRepoWithRemote(t)
	// The remote is gone, so the pull cannot work. The branch still tracks
	// it, so the picker tries anyway.
	runGitIn(t, repo, "remote", "set-url", "origin", "C:/nonexistent/nowhere")

	output, input, results, failures := runPicker(t, repo)
	defer input.Close()

	time.Sleep(400 * time.Millisecond)

	// Enter switches to the highlighted branch and pulls it.
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}

	// The failure is shown in full, and the picker waits.
	waitFor(t, "the error to be shown", func() bool {
		return strings.Contains(output.String(), "git pull failed")
	})
	waitFor(t, "the error panel", func() bool {
		return strings.Contains(output.String(), "esc exits")
	})

	// Escape leaves, and the picker ends by itself rather than sitting
	// there with an error on screen.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}

	var final tea.Model
	select {
	case final = <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the picker did not end after the failure")
	}

	m, ok := final.(model)
	if !ok {
		t.Fatalf("unexpected final model %T", final)
	}
	if m.fatal == nil {
		t.Fatal("the failure should have been recorded")
	}
	if m.fatal.Code == 0 {
		t.Fatal("the exit code should have been kept")
	}
	// The exit code is git's own, for the same command.
	want := gitExitCode(t, repo, "pull")
	if m.fatal.Code != want {
		t.Fatalf("the exit code: got %d, want git's %d", m.fatal.Code, want)
	}
	// Git words this differently between versions, so the check is on the
	// parts that do not move.
	if !strings.Contains(m.fatal.Detail, "fatal:") {
		t.Fatalf("the whole error should be kept:\n%s", m.fatal.Detail)
	}
}

func TestAPushToAPlainRemoteAsksNothing(t *testing.T) {
	withHome(t)

	// A remote that is a folder on disk is not GitHub, so there is no
	// pull request page to open.
	repo, _ := makeTestRepoWithRemote(t)

	output, input, results, failures := runPicker(t, repo)
	defer input.Close()

	time.Sleep(400 * time.Millisecond)

	// Ctrl+P pushes the current branch.
	if _, err := input.Write([]byte{0x10}); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "the push output", func() bool {
		return strings.Contains(output.String(), "esc closes the output")
	})

	// Closing the output brings the picker back, and there is no pull
	// request page to ask about.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the list to come back", func() bool {
		return strings.Contains(output.String(), "Pushed main")
	})

	if strings.Contains(output.String(), "Open the pull request page?") {
		t.Fatalf("there is no pull request page to open:\n%s", visible(output.String()))
	}

	// The picker carries on.
	select {
	case final := <-results:
		t.Fatalf("the picker closed itself after pushing: %v", final)
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	default:
	}

	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("escape did not close the picker")
	}
}

func TestThePickerClonesIntoTheCurrentDirectory(t *testing.T) {
	withHome(t)

	// A repository to clone from, and a folder with no repository in it
	// to clone into.
	_, remote := makeTestRepoWithRemote(t)
	dir := t.TempDir()

	output, input, results, failures := runPickerWithoutRepo(t, dir)
	defer input.Close()

	time.Sleep(400 * time.Millisecond)

	// There is no repository, and the picker says so.
	waitFor(t, "the empty state", func() bool {
		return strings.Contains(output.String(), "not inside a git repository")
	})

	// Ctrl+G asks for the address to clone.
	if _, err := input.Write([]byte{0x07}); err != nil { // ctrl+g
		t.Fatal(err)
	}
	waitFor(t, "the clone prompt", func() bool {
		return strings.Contains(output.String(), "Clone:")
	})

	// The address is typed and confirmed.
	if _, err := input.Write([]byte(remote)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}

	// The clone runs, and what git said about it is on screen.
	waitFor(t, "the clone output", func() bool {
		return strings.Contains(output.String(), "esc closes the output")
	})

	// The folder is there, with the branches in it.
	folder := filepath.Join(dir, repoFolder(remote))
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

	// And the picker is in the new repository.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the new repository to be shown", func() bool {
		return strings.Contains(output.String(), repoFolder(remote))
	})

	// Escape leaves.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("escape did not close the picker")
	}
}

func TestPickerCreatesABranchAndStaysOpen(t *testing.T) {
	repo := makeTestRepo(t)
	output, input, results, failures := runPicker(t, repo)
	defer input.Close()

	time.Sleep(400 * time.Millisecond)

	// Ctrl+N asks for a name, then the name is typed and confirmed.
	if _, err := input.Write([]byte("\x0e")); err != nil { // ctrl+n
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := input.Write([]byte("my-branch")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := input.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}

	// The branch is made, and what git said about it is on screen.
	waitFor(t, "the branch to be created", func() bool {
		return strings.Contains(output.String(), "esc closes the output")
	})

	// Closing the output brings the picker back.
	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the list to come back", func() bool {
		return strings.Contains(output.String(), "Created and switched to my-branch")
	})

	select {
	case final := <-results:
		t.Fatalf("the picker closed itself after creating a branch: %v", final)
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	default:
	}

	if got := currentBranch(repo); got != "my-branch" {
		t.Fatalf("the repo should be on the new branch, got %q", got)
	}

	if _, err := input.Write([]byte("\x1b")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-results:
	case err := <-failures:
		t.Fatalf("the picker failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("escape did not close the picker")
	}
}
