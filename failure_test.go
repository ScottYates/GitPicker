package main

// Tests for the failure paths: what git said, what the picker does about it
// and what the process ends with.
//
// The rule the tests hold to is that a failure the user asked for ends the
// picker with the exit code git gave, and that a background fetch failing
// does not.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitErr is a GitError, or a failure saying it was not one.
func gitErr(t *testing.T, err error) *GitError {
	t.Helper()
	if err == nil {
		t.Fatal("the failure was not reported")
	}
	gitErr, ok := err.(*GitError)
	if !ok {
		t.Fatalf("the failure was not a GitError: %v", err)
	}
	return gitErr
}

func TestCheckGitPassesStdoutOnSuccess(t *testing.T) {
	out, err := checkGit("a branch\n", "", 0, "for-each-ref")
	if err != nil {
		t.Fatalf("a command that worked should not be an error: %v", err)
	}
	if out != "a branch\n" {
		t.Fatalf("stdout: got %q", out)
	}
}

func TestCheckGitCarriesTheExitCode(t *testing.T) {
	_, err := checkGit("", "fatal: nope\n", 128, "pull")
	got := gitErr(t, err)

	if got.Code != 128 {
		t.Fatalf("the exit code: got %d, want 128", got.Code)
	}
	if got.Message != "git pull failed: fatal: nope" {
		t.Fatalf("the summary: got %q", got.Message)
	}
}

func TestCheckGitKeepsBothStreams(t *testing.T) {
	// A merge conflict writes the conflict lines to stdout and the failure
	// to stderr, so either stream on its own misses half of it.
	_, err := checkGit(
		"Auto-merging file\nCONFLICT (content): Merge conflict in file\n",
		"error: Automatic merge failed\n",
		1,
		"pull",
	)
	got := gitErr(t, err)

	if !strings.Contains(got.Detail, "CONFLICT (content): Merge conflict in file") {
		t.Fatalf("the stdout should be kept in full, got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "error: Automatic merge failed") {
		t.Fatalf("the stderr should be kept in full, got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "stdout:") || !strings.Contains(got.Detail, "stderr:") {
		t.Fatalf("the streams should be labelled, got %q", got.Detail)
	}
}

func TestCheckGitFallsBackToStdout(t *testing.T) {
	_, err := checkGit("something went wrong\n", "", 1, "push")
	got := gitErr(t, err)

	if got.Message != "git push failed: something went wrong" {
		t.Fatalf("the summary: got %q", got.Message)
	}
	if strings.Contains(got.Detail, "stderr:") {
		t.Fatalf("a stream that said nothing should be left out, got %q", got.Detail)
	}
}

func TestAFailedFetchCarriesTheGitExitCode(t *testing.T) {
	// A remote that is not there cannot be fetched from, and git says so
	// with its own exit code. A repository with no remote at all would
	// fetch cleanly, because there is nothing to fetch.
	repo, _ := makeTestRepoWithRemote(t)
	runGitIn(t, repo, "remote", "set-url", "origin", "C:/nonexistent/nowhere")

	_, err := fetch(repo)
	got := gitErr(t, err)

	if got.Code == 0 {
		t.Fatal("a failed fetch should carry an exit code")
	}
	if got.Detail == "" {
		t.Fatal("a failed fetch should say what went wrong")
	}
}

func TestAFailedFetchDoesNotEndThePicker(t *testing.T) {
	// The fetch runs on its own in the background. It fails whenever the
	// machine is offline, so it must not take the picker down with it.
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(fetchDoneMsg{fetchResult{err: &GitError{Code: 128, Message: "git fetch failed"}}})
	m = next.(model)

	if m.fatal != nil {
		t.Fatalf("a failed fetch should not be fatal, got %v", m.fatal)
	}
	if m.phase != phaseList {
		t.Fatalf("the picker should carry on, got phase %d", m.phase)
	}
}

func TestAFailedPullEndsThePicker(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(pullDoneMsg{branch: "master", err: &GitError{Code: 1, Message: "git pull failed"}})
	m = next.(model)

	if m.phase != phaseError {
		t.Fatalf("a failed pull should end the picker, got phase %d", m.phase)
	}
	if m.fatal == nil {
		t.Fatal("the failure should be recorded")
	}
	if m.fatal.Code != 1 {
		t.Fatalf("the exit code should be kept, got %d", m.fatal.Code)
	}
}

func TestAFailedSwitchEndsThePicker(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(switchDoneMsg{branch: "master", err: &GitError{Code: 128, Message: "git switch failed"}})
	m = next.(model)

	if m.phase != phaseError {
		t.Fatalf("a failed switch should end the picker, got phase %d", m.phase)
	}
	if m.fatal == nil || m.fatal.Code != 128 {
		t.Fatalf("the exit code should be kept, got %v", m.fatal)
	}
}

func TestAFailedPushEndsThePicker(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(pushDoneMsg{err: &GitError{Code: 128, Message: "git push failed"}})
	m = next.(model)

	if m.phase != phaseError {
		t.Fatalf("a failed push should end the picker, got phase %d", m.phase)
	}
	if m.fatal == nil || m.fatal.Code != 128 {
		t.Fatalf("the exit code should be kept, got %v", m.fatal)
	}
}

func TestAFailedFileListingEndsThePicker(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(filesLoadedMsg{err: &GitError{Code: 128, Message: "git status failed"}})
	m = next.(model)

	if m.phase != phaseError {
		t.Fatalf("a failed status should end the picker, got phase %d", m.phase)
	}
}

func TestAPlainFailureStillExitsWithAFailureCode(t *testing.T) {
	// A fault in the picker itself has no git exit code, so it exits 1
	// rather than 0.
	m := testModel("master")

	next, _ := m.fail(os.ErrInvalid)
	m = next.(model)

	if m.fatal == nil {
		t.Fatal("the failure should be recorded")
	}
	if m.fatal.Code != 1 {
		t.Fatalf("the exit code: got %d, want 1", m.fatal.Code)
	}
	if m.fatal.Detail == "" {
		t.Fatal("the failure should say what went wrong")
	}
}

func TestTheErrorPanelShowsTheWholeError(t *testing.T) {
	m := testModel("master")
	m.width = 60
	m.height = 24
	m.fatal = &GitError{
		Code:    1,
		Message: "git pull failed: fatal: Merge aborted",
		Detail:  "stdout:\nAuto-merging file\nCONFLICT (content): Merge conflict in file\n\nstderr:\nerror: Automatic merge failed\n",
	}

	panel := visible(m.viewError())
	t.Logf("the panel:\n%s", panel)

	if !strings.Contains(panel, "git pull failed: fatal: Merge aborted") {
		t.Fatalf("the summary should be on screen:\n%s", panel)
	}
	if !strings.Contains(panel, "CONFLICT (content): Merge conflict in file") {
		t.Fatalf("the whole error should be on screen:\n%s", panel)
	}
	if !strings.Contains(panel, "error: Automatic merge failed") {
		t.Fatalf("the whole error should be on screen:\n%s", panel)
	}
	if !strings.Contains(panel, "esc exits") {
		t.Fatalf("the panel should say how to leave:\n%s", panel)
	}
}

func TestTheErrorPanelWrapsALongLine(t *testing.T) {
	m := testModel("master")
	m.width = 40
	m.height = 24
	m.fatal = &GitError{
		Code:    1,
		Message: "git pull failed",
		Detail:  "stderr:\n" + strings.Repeat("a long error line ", 10),
	}

	panel := visible(m.viewError())

	for _, line := range strings.Split(panel, "\n") {
		// The panel is measured in cells and not in bytes, because the
		// line drawing characters take more than one byte each.
		if displayWidth(line) > 40 {
			t.Fatalf("the panel is wider than the screen: %q", line)
		}
	}
	if !strings.Contains(panel, "a long error line") {
		t.Fatalf("the error should be on screen:\n%s", panel)
	}
}

func TestTheErrorPanelKeepsEveryLine(t *testing.T) {
	m := testModel("master")
	m.width = 60
	m.height = 10

	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("a line of the error\n")
	}
	m.fatal = &GitError{Code: 1, Message: "git pull failed", Detail: b.String()}

	// Nothing is left out of the error, because the panel scrolls through
	// it rather than cutting it short.
	if got := len(m.errorLines(58)); got != 41 {
		t.Fatalf("the panel holds %d lines, want 41", got)
	}

	panel := visible(m.viewError())
	if strings.Contains(panel, "more lines, see the terminal") {
		t.Fatalf("the panel should not say what was left out:\n%s", panel)
	}
	// The panel shows the first page of the error.
	if !strings.Contains(panel, "a line of the error") {
		t.Fatalf("the error should be on screen:\n%s", panel)
	}
}

func TestEscapeOnTheErrorPanelQuitsThePicker(t *testing.T) {
	m := testModel("master")
	m.phase = phaseError
	m.fatal = &GitError{Code: 1, Message: "git pull failed"}

	next, cmd := m.handleKey(keyMsg("esc"))
	m = next.(model)

	if cmd == nil {
		t.Fatal("escape should quit the picker")
	}
	if m.fatal == nil {
		t.Fatal("the failure should survive the quit, so it can be reported")
	}
}

func TestAStrayKeyDoesNotQuitTheErrorPanel(t *testing.T) {
	// The console pastes the clipboard when the mouse is clicked in the
	// window, so a key that arrives on its own should not close the panel
	// while the error is being read.
	m := testModel("master")
	m.phase = phaseError
	m.fatal = &GitError{Code: 1, Message: "git pull failed"}

	next, cmd := m.handleKey(keyMsg("x"))
	m = next.(model)

	if cmd != nil {
		t.Fatal("a stray key should not quit the picker")
	}
	if m.phase != phaseError {
		t.Fatalf("the panel should still be up, got phase %d", m.phase)
	}
}

// TestHelperProcess is not a test of its own. It is the entry point the
// exit code test runs, so the thing that exits is the real program.
//
// The arguments are the test binary's own, so the app is run with none of
// them.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GP_APP") != "1" {
		t.Skip("this is the helper for the exit code test")
	}
	os.Exit(mainExitCode(nil, os.Stdout, os.Stderr))
}

// brokenRepo is a directory that looks like a repository but is not one.
//
// The .git file points at a gitdir that is not there, so git refuses to
// work in it and exits with 128.
func brokenRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitfile := filepath.Join(dir, ".git")
	if err := os.WriteFile(gitfile, []byte("gitdir: C:/nonexistent/nowhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// gitExitCode is the exit code git gives for a command that fails.
func gitExitCode(t *testing.T, dir string, args ...string) int {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		t.Fatalf("git %v was expected to fail", args)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("git %v: %v", args, err)
	}
	return exitErr.ExitCode()
}

func TestTheProcessExitsWithGitsExitCode(t *testing.T) {
	// This is the whole point of the feature: a script that runs the
	// picker has to be able to see that it failed, and why.
	repo := brokenRepo(t)

	// The code to expect is git's own, for the same command. Git words
	// this differently between versions and platforms, and the exit code
	// is the part that has to agree.
	want := gitExitCode(t, repo, "for-each-ref", "--format=%(refname)")

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GP_APP=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("the app could not be run: %v", err)
	}

	if code != want {
		t.Fatalf("the exit code: got %d, want git's %d\n%s", code, want, stderr.String())
	}
	if !strings.Contains(stderr.String(), "git for-each-ref failed") {
		t.Fatalf("the error should be reported in full:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "fatal:") {
		t.Fatalf("git's own words should be reported:\n%s", stderr.String())
	}
}

func TestTheProcessExitsWithZeroForAVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := mainExitCode([]string{"--version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("the exit code: got %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "git-picker") {
		t.Fatalf("the version should be printed:\n%s", stdout.String())
	}
}

func TestTheProcessExitsWithOneForANonGitFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := mainExitCode([]string{"--dump-labels"}, &stdout, &stderr)

	// The working directory is the project, which is a repository, so this
	// one succeeds. The point is that a plain failure is 1, so check the
	// reporting helper directly instead.
	if code != 0 {
		t.Fatalf("the exit code: got %d, want 0", code)
	}

	var out bytes.Buffer
	if got := reportError(os.ErrInvalid, &out); got != 1 {
		t.Fatalf("a plain failure should exit 1, got %d", got)
	}
	if !strings.Contains(out.String(), "invalid argument") {
		t.Fatalf("the error should be reported:\n%s", out.String())
	}
}

func TestTheProcessReportsTheWholeError(t *testing.T) {
	var out bytes.Buffer

	code := reportError(&GitError{
		Code:    1,
		Message: "git pull failed: fatal: Merge aborted",
		Detail:  "stdout:\nCONFLICT (content): Merge conflict in file\n\nstderr:\nerror: Automatic merge failed\n",
	}, &out)

	if code != 1 {
		t.Fatalf("the exit code: got %d, want 1", code)
	}
	text := out.String()
	if !strings.Contains(text, "git pull failed: fatal: Merge aborted") {
		t.Fatalf("the summary should be reported:\n%s", text)
	}
	if !strings.Contains(text, "CONFLICT (content): Merge conflict in file") {
		t.Fatalf("the whole error should be reported:\n%s", text)
	}
	if !strings.Contains(text, "error: Automatic merge failed") {
		t.Fatalf("the whole error should be reported:\n%s", text)
	}
}
