package main

// Tests for the git layer: ref parsing, branch names and the repo root
// walk.

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBranchNameFromRef(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"refs/heads/master", "master"},
		{"refs/heads/feature/x", "feature/x"},
		{"refs/remotes/origin/master", "master"},
		{"refs/remotes/origin/feature/x", "feature/x"},
		{"refs/tags/v1.0", "refs/tags/v1.0"},
	}
	for _, c := range cases {
		if got := branchNameFromRef(c.ref); got != c.want {
			t.Errorf("branchNameFromRef(%q): got %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestParseRefLine(t *testing.T) {
	cases := []struct {
		line      string
		ref       string
		timestamp int64
		upstream  string
	}{
		{"refs/heads/master\t1700000000\torigin/master", "refs/heads/master", 1700000000, "origin/master"},
		{"refs/heads/master\t1700000000\t", "refs/heads/master", 1700000000, ""},
		{"refs/remotes/origin/x\t1700000001\t", "refs/remotes/origin/x", 1700000001, ""},
		{"refs/heads/master", "refs/heads/master", 0, ""},
	}
	for _, c := range cases {
		ref, timestamp, upstream := parseRefLine(c.line)
		if ref != c.ref || timestamp != c.timestamp || upstream != c.upstream {
			t.Errorf("parseRefLine(%q): got (%q, %d, %q), want (%q, %d, %q)",
				c.line, ref, timestamp, upstream, c.ref, c.timestamp, c.upstream)
		}
	}
}

func TestParseTimestampIgnoresRubbish(t *testing.T) {
	if got := parseTimestamp("not a number"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := parseTimestamp(" 1700000000 "); got != 1700000000 {
		t.Fatalf("expected 1700000000, got %d", got)
	}
}

// makeTestRepo creates a git repo with a couple of branches and returns its
// path.
func makeTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "first")
	run("branch", "feature/x")
	run("branch", "master")
	return repo
}

func TestFindRepoRoot(t *testing.T) {
	repo := makeTestRepo(t)

	// The root comes back in its long form, because find_repo_root expands
	// an 8.3 alias. On a machine where the temporary folder has an alias,
	// such as a CI runner, the path handed in is the short one.
	want := longPath(repo)

	root, ok := findRepoRoot(repo)
	if !ok {
		t.Fatal("the repo root was not found")
	}
	if root != want {
		t.Fatalf("repo root: got %q, want %q", root, want)
	}

	// From a subdirectory it walks up to the root.
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	root, ok = findRepoRoot(nested)
	if !ok || root != want {
		t.Fatalf("walk up from a subdirectory: got %q (%v), want %q", root, ok, want)
	}
}

func TestFindRepoRootInANonRepo(t *testing.T) {
	dir := t.TempDir()
	if _, ok := findRepoRoot(dir); ok {
		t.Fatal("a plain directory should not be reported as a repo")
	}
}

func TestListBranches(t *testing.T) {
	repo := makeTestRepo(t)

	branches, err := listBranches(repo)
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, b := range branches {
		names[b.Name] = true
	}
	for _, want := range []string{"main", "master", "feature/x"} {
		if !names[want] {
			t.Errorf("branch %q was not listed", want)
		}
	}
	for _, b := range branches {
		if b.Remote {
			t.Errorf("branch %q should not be marked remote", b.Name)
		}
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := makeTestRepo(t)
	if got := currentBranch(repo); got != "main" {
		t.Fatalf("current branch: got %q, want %q", got, "main")
	}
}

func TestHeadFromGitDir(t *testing.T) {
	repo := makeTestRepo(t)
	if got := headFromGitDir(repo); got != "refs/heads/main" {
		t.Fatalf("HEAD: got %q, want %q", got, "refs/heads/main")
	}
	// A detached head holds a commit id, so there is no ref to report.
	run := exec.Command("git", "checkout", "-q", "--detach", "HEAD")
	run.Dir = repo
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("git detach: %v\n%s", err, out)
	}
	if got := headFromGitDir(repo); got != "" {
		t.Fatalf("a detached head should report no ref, got %q", got)
	}
	// And the fallback still gives the right answer for the detached case.
	if got := currentBranch(repo); got != "" {
		t.Fatalf("a detached head should report no branch, got %q", got)
	}
}

func TestHeadFromGitDirOutsideARepo(t *testing.T) {
	if got := headFromGitDir(t.TempDir()); got != "" {
		t.Fatalf("expected no ref, got %q", got)
	}
}

func TestLongPathExpandsAnEightDotThreeAlias(t *testing.T) {
	// A folder name long enough that Windows gives it a short alias.
	dir := t.TempDir()
	long := filepath.Join(dir, "BTS-MAIN-2026-Q3-archive-migration")
	if err := os.MkdirAll(long, 0o755); err != nil {
		t.Fatal(err)
	}

	alias := shortPath(long)
	if alias == "" || alias == long {
		t.Skipf("this machine does not give the folder an 8.3 alias: %q", alias)
	}

	want := longPath(long)
	if got := longPath(alias); got != want {
		t.Fatalf("longPath(%q): got %q, want %q", alias, got, want)
	}
}

func TestSwitchArgs(t *testing.T) {
	local := Branch{Name: "master"}
	remote := Branch{Name: "feature/x", Remote: true}

	got := switchArgs(local)
	if len(got) != 2 || got[0] != "switch" || got[1] != "master" {
		t.Fatalf("switch args for a local branch: %v", got)
	}
	got = switchArgs(remote)
	if len(got) != 3 || got[0] != "switch" || got[1] != "--track" || got[2] != "origin/feature/x" {
		t.Fatalf("switch args for a remote branch: %v", got)
	}
}

func TestValidBranchName(t *testing.T) {
	if err := validBranchName("feature/x"); err != nil {
		t.Fatalf("a valid name was rejected: %v", err)
	}
	for _, bad := range []string{"-dash", "with space", "ti~lde", "caret^", "co:lon", "qu?mark", "star*", "bra[cket", "back\\slash"} {
		if err := validBranchName(bad); err == nil {
			t.Errorf("the invalid name %q was accepted", bad)
		}
	}
}

func TestSwitchCmdSwitchesAndCreatesABranch(t *testing.T) {
	repo := makeTestRepo(t)

	msg := switchCmd(repo, switchAction{branch: Branch{Name: "master"}, newName: "my-branch"})()
	done, ok := msg.(switchDoneMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	if done.err != nil {
		t.Fatalf("the switch failed: %v", done.err)
	}
	if done.newName != "my-branch" {
		t.Fatalf("new branch: got %q, want %q", done.newName, "my-branch")
	}
	if got := currentBranch(repo); got != "my-branch" {
		t.Fatalf("the repo should be on the new branch, got %q", got)
	}
}

func TestSwitchCmdReportsAPullFailureWithoutFailing(t *testing.T) {
	repo := makeTestRepo(t)

	msg := switchCmd(repo, switchAction{branch: Branch{Name: "master"}})()
	done, ok := msg.(switchDoneMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	// The branch has no upstream, so the pull fails. The switch worked, so
	// that is reported on its own and the picker stays open.
	if done.err != nil {
		t.Fatalf("the switch itself should not have failed: %v", done.err)
	}
	if done.pullErr == nil {
		t.Fatal("the pull failure should have been reported")
	}
	if done.branch != "master" {
		t.Fatalf("branch: got %q, want %q", done.branch, "master")
	}
	if got := currentBranch(repo); got != "master" {
		t.Fatalf("the repo should be on master, got %q", got)
	}
}
