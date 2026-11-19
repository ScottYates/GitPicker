package main

// End to end tests that drive the real picker.
//
// The picker is run with a pipe for input and a buffer for output, so the
// whole program is exercised: the keys, the git work, the refresh of the
// list and the decision to stay open or to leave.

import (
	"bytes"
	"os"
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

	m := newModel(repo, "repo", currentBranch(repo), branches, fetchCh)

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
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
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

	waitFor(t, "the switch to finish", func() bool {
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

	waitFor(t, "the branch to be created", func() bool {
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
