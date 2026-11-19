package main

// Tests for the pull request page: the address that is opened, the branch
// the request goes back to, and the question about opening it.

import (
	"strings"
	"testing"
)

// githubPushOutput is what git prints at the end of a push to GitHub when
// the branch has no pull request yet.
const githubPushOutput = `To github.com:owner/repo.git
   8f4c1a2..9b7d3e4  feature/pr -> feature/pr

remote:
remote: Create a pull request for 'feature/pr' on GitHub by visiting:
remote:      https://github.com/owner/repo/pull/new/feature/pr
remote:
`

// githubExistingPushOutput is what git prints when the branch already has a
// pull request.
const githubExistingPushOutput = `To github.com:owner/repo.git
   8f4c1a2..9b7d3e4  feature/pr -> feature/pr

remote:
remote: View the pull request for 'feature/pr' on GitHub by visiting:
remote:      https://github.com/owner/repo/pull/42
remote:
`

// githubPushOutputFor is the output git prints at the end of a push to
// GitHub for a branch.
func githubPushOutputFor(branch string) string {
	return strings.ReplaceAll(githubPushOutput, "feature/pr", branch)
}

// makeTestRepoWithDevelop is a repository with a remote that has a develop
// branch on it, and a feature branch that was branched from develop.
func makeTestRepoWithDevelop(t *testing.T) string {
	t.Helper()
	repo, _ := makeTestRepoWithRemote(t)

	runGitIn(t, repo, "branch", "develop")
	runGitIn(t, repo, "push", "-q", "origin", "develop")
	runGitIn(t, repo, "switch", "-q", "-c", "feature/pr", "develop")

	return repo
}

func TestTheAddressIsTakenFromWhatGitPrinted(t *testing.T) {
	repo := makeTestRepoWithDevelop(t)

	got := pullRequestURL(repo, githubPushOutput)

	want := "https://github.com/owner/repo/compare/develop...feature/pr?quick_pull=1"
	if got != want {
		t.Fatalf("the address: got %q, want %q", got, want)
	}
}

func TestAnExistingPullRequestIsOpenedAsItIs(t *testing.T) {
	// The pull request already exists, so its own address is the one to
	// open, base branch and all.
	repo := makeTestRepoWithDevelop(t)

	got := pullRequestURL(repo, githubExistingPushOutput)

	want := "https://github.com/owner/repo/pull/42"
	if got != want {
		t.Fatalf("the address: got %q, want %q", got, want)
	}
}

func TestANonGitHubRemoteIsNotOffered(t *testing.T) {
	repo := makeTestRepoWithDevelop(t)

	output := strings.ReplaceAll(
		githubPushOutput,
		"https://github.com/owner/repo",
		"https://gitlab.com/owner/repo",
	)

	if got := pullRequestURL(repo, output); got != "" {
		t.Fatalf("a remote that is not GitHub should not be offered, got %q", got)
	}
}

func TestAPushWithNothingToOpenIsNotOffered(t *testing.T) {
	repo := makeTestRepoWithDevelop(t)

	// A push to a plain folder on disk says nothing about a pull request.
	output := "To /tmp/repo\n   8f4c1a2..9b7d3e4  feature/pr -> feature/pr\n"

	if got := pullRequestURL(repo, output); got != "" {
		t.Fatalf("there should be nothing to open, got %q", got)
	}
}

func TestTheBaseIsTheBranchTheWorkWasBranchedFrom(t *testing.T) {
	// The branch was made from develop, so the pull request goes back to
	// develop and not to the default branch.
	repo := makeTestRepoWithDevelop(t)

	base := pullRequestBase(repo, "feature/pr")

	if base != "develop" {
		t.Fatalf("the base: got %q, want %q", base, "develop")
	}
}

func TestTheBaseIsFoundWhenTheBranchWasCreatedFromHead(t *testing.T) {
	// A branch created with no starting point is recorded as "Created from
	// HEAD", and the HEAD reflog says which branch it was made from.
	repo := makeTestRepoWithDevelop(t)
	runGitIn(t, repo, "switch", "-q", "develop")
	runGitIn(t, repo, "switch", "-q", "-c", "feature/y")

	base := pullRequestBase(repo, "feature/y")

	if base != "develop" {
		t.Fatalf("the base: got %q, want %q", base, "develop")
	}
}

func TestTheBaseIsLeftOutWhenItIsNotOnTheRemote(t *testing.T) {
	// A pull request can only be made against a branch that is in the
	// repository, so a branch that is only local is no use.
	repo := makeTestRepoWithDevelop(t)
	runGitIn(t, repo, "switch", "-q", "-c", "feature/z", "develop")
	runGitIn(t, repo, "branch", "local-only")

	base := pullRequestBase(repo, "feature/z")
	if base != "develop" {
		t.Fatalf("the base: got %q, want %q", base, "develop")
	}

	// A branch made from one that never went to the remote has no base to
	// offer.
	runGitIn(t, repo, "switch", "-q", "-c", "feature/w", "local-only")
	if got := pullRequestBase(repo, "feature/w"); got != "" {
		t.Fatalf("a base that is not on the remote should be left out, got %q", got)
	}
}

func TestTheAddressIsLeftAloneWhenThereIsNoBase(t *testing.T) {
	// Without a base the address git printed is used, which leaves GitHub
	// to pick its default.
	repo := makeTestRepoWithDevelop(t)
	runGitIn(t, repo, "branch", "local-only")
	runGitIn(t, repo, "switch", "-q", "-c", "feature/w", "local-only")

	got := pullRequestURL(repo, githubPushOutputFor("feature/w"))

	if !strings.HasPrefix(got, "https://github.com/owner/repo/pull/new/") {
		t.Fatalf("the address should be the one git printed, got %q", got)
	}
}

func TestAPushWithAPullRequestAsksAboutOpeningIt(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(pushDoneMsg{prURL: "https://github.com/owner/repo/pull/42"})
	m = next.(model)

	if m.phase != phasePRConfirm {
		t.Fatalf("the picker should ask about the page, got phase %d", m.phase)
	}
	if m.prURL != "https://github.com/owner/repo/pull/42" {
		t.Fatalf("the address should be kept, got %q", m.prURL)
	}
	if m.prChoice != 0 {
		t.Fatal("yes should be the starting answer")
	}
}

func TestAPushWithNothingToOpenAsksNothing(t *testing.T) {
	m := testModel("master")
	m.phase = phaseList

	next, _ := m.Update(pushDoneMsg{})
	m = next.(model)

	if m.phase != phaseList {
		t.Fatalf("there is nothing to open, so nothing should be asked, got phase %d", m.phase)
	}
}

func TestTheBrowserIsOpenedWithTheAddress(t *testing.T) {
	var opened []string
	old := openBrowser
	openBrowser = func(address string) error {
		opened = append(opened, address)
		return nil
	}
	t.Cleanup(func() { openBrowser = old })

	m := testModel("master")
	m.phase = phasePRConfirm
	m.prURL = "https://github.com/owner/repo/pull/42"

	next, cmd := m.handleKey(keyMsg("y"))
	m = next.(model)

	if cmd == nil {
		t.Fatal("yes should have started opening the page")
	}
	if _, ok := cmd().(browserDoneMsg); !ok {
		t.Fatal("opening the page should report back")
	}

	if len(opened) != 1 {
		t.Fatalf("the browser should have been asked once, got %d", len(opened))
	}
	if opened[0] != "https://github.com/owner/repo/pull/42" {
		t.Fatalf("the address: got %q", opened[0])
	}
	if m.phase != phaseList {
		t.Fatalf("the picker should be back on the list, got phase %d", m.phase)
	}
}

func TestAnsweringNoOpensNothing(t *testing.T) {
	var opened []string
	old := openBrowser
	openBrowser = func(address string) error {
		opened = append(opened, address)
		return nil
	}
	t.Cleanup(func() { openBrowser = old })

	m := testModel("master")
	m.phase = phasePRConfirm
	m.prURL = "https://github.com/owner/repo/pull/42"

	next, cmd := m.handleKey(keyMsg("n"))
	m = next.(model)

	if cmd != nil {
		t.Fatal("no should not have started anything")
	}
	if len(opened) != 0 {
		t.Fatalf("the browser should not have been asked, got %v", opened)
	}
	if m.phase != phaseList {
		t.Fatalf("the picker should be back on the list, got phase %d", m.phase)
	}
}

func TestTheQuestionCanBeAnsweredWithTheArrowKeys(t *testing.T) {
	m := testModel("master")
	m.phase = phasePRConfirm
	m.prURL = "https://github.com/owner/repo/pull/42"

	next, _ := m.handleKey(keyMsg("down"))
	m = next.(model)
	if m.prChoice != 1 {
		t.Fatalf("down should have moved to no, got %d", m.prChoice)
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

func TestTheQuestionShowsTheAddress(t *testing.T) {
	m := testModel("master")
	m.width = 60
	m.phase = phasePRConfirm
	m.prURL = "https://github.com/owner/repo/pull/42"

	panel := visible(m.viewPRConfirm())
	t.Logf("the question:\n%s", panel)

	if !strings.Contains(panel, "Open the pull request page?") {
		t.Fatalf("the question should be on screen:\n%s", panel)
	}
	if !strings.Contains(panel, "https://github.com/owner/repo/pull/42") {
		t.Fatalf("the address should be on screen:\n%s", panel)
	}
}
