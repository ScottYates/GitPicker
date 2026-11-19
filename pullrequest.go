package main

// The pull request page.
//
// Git prints the address of a pull request at the end of a push to GitHub,
// and the picker offers to open it. The address comes from what git
// printed, so a GitHub Enterprise host works as well as github.com.
//
// The branch the pull request should go back to is the branch the work was
// branched from, rather than the default branch of the repository. Git
// records that in the reflog of the branch.

import (
	"net/url"
	"strconv"
	"strings"
)

// pullRequestURL is the address of the pull request page for the branch
// that was pushed.
//
// When the branch already has a pull request, git prints the address of it
// and that is used as it is. When there is not, git prints the address of
// the page that creates one, and that is rebuilt with the branch the work
// was branched from, so the pull request goes back where it came from
// rather than to the default branch.
//
// It is "" when the push was not to GitHub, or when there was nothing to
// open.
func pullRequestURL(root string, output string) string {
	printed := printedPullURL(output)
	if printed == "" {
		return ""
	}

	host, path := splitURL(printed)
	if !strings.Contains(host, "github") {
		return ""
	}

	const marker = "/pull/new/"
	i := strings.Index(path, marker)
	if i < 0 {
		// The branch already has a pull request, and that is the page.
		return printed
	}

	head := path[i+len(marker):]
	base := pullRequestBase(root, head)
	if base == "" {
		return printed
	}

	// The compare form takes the base and the head, and takes you
	// straight to the page that opens the pull request.
	prefix := path[:i]
	return "https://" + host + "/" + prefix + "/compare/" +
		urlBranch(base) + "..." + urlBranch(head) + "?quick_pull=1"
}

// printedPullURL is the pull request address git printed at the end of a
// push.
//
// Git writes it like this:
//
//	remote: Create a pull request for 'main' on GitHub by visiting:
//	remote:      https://github.com/owner/repo/pull/new/main
//
// The "remote:" prefix is how the output of the remote reaches the
// terminal, so it is stripped off before the address is used.
func printedPullURL(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "remote:"))
		if !strings.HasPrefix(line, "https://") {
			continue
		}
		if !strings.Contains(line, "/pull/") {
			continue
		}
		return line
	}
	return ""
}

// splitURL is the host and the path of an address.
func splitURL(address string) (string, string) {
	rest := strings.TrimPrefix(address, "https://")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, ""
}

// urlBranch is a branch name as it goes into a url path.
//
// A slash is left as it is, because that is how GitHub writes branch names
// in the links it makes.
func urlBranch(name string) string {
	return strings.ReplaceAll(url.PathEscape(name), "%2F", "/")
}

// pullRequestBase is the branch a pull request should be made against.
//
// It is the branch the work was branched from, so the request goes back
// where it came from rather than to the default branch of the repository.
//
// The branch has to be on the remote for GitHub to accept it, so a branch
// that is not there is left out and GitHub picks its default.
func pullRequestBase(root string, head string) string {
	remotes := remoteBranches(root)

	base := branchBase(root, head, remotes)
	if base == "" || base == head {
		return ""
	}
	if !remotes[base] {
		return ""
	}
	return base
}

// branchBase is the branch a branch was branched from.
//
// Git records it in the reflog of the branch, as "branch: Created from
// main". A branch created with no starting point is recorded as "Created
// from HEAD", and then the HEAD reflog says which branch was checked out
// at the time.
//
// The branch may be recorded as a remote tracking reference, such as
// origin/main, and the name in the repository is what is wanted, because
// that is what a pull request is made against.
func branchBase(root string, name string, remotes map[string]bool) string {
	entries := readReflog(root, name)
	if len(entries) == 0 {
		return ""
	}

	// The creation is the oldest entry.
	created := entries[len(entries)-1]
	// Git writes it as "branch: Created from main".
	const marker = "Created from "
	i := strings.Index(created.what, marker)
	if i < 0 {
		return ""
	}

	from := strings.TrimSpace(created.what[i+len(marker):])
	if from == "HEAD" {
		from = checkedOutBefore(root, name, created.when)
	}
	if from == "" {
		return ""
	}

	if remotes[from] {
		return from
	}
	if i := strings.Index(from, "/"); i > 0 {
		if short := from[i+1:]; remotes[short] {
			return short
		}
	}
	return ""
}

// checkedOutBefore is the branch that was checked out when a branch was
// created.
//
// A branch created with no starting point is recorded as "Created from
// HEAD", and the HEAD reflog says which branch was checked out at the
// time. The entry is the one made at that moment and onto that branch, so
// an earlier or a later visit to the same branch is not mistaken for it.
func checkedOutBefore(root string, name string, when int64) string {
	var found bool
	var best reflogEntry
	for _, entry := range readReflog(root, "HEAD") {
		if entry.when > when {
			continue
		}
		_, to, ok := checkoutEntry(entry.what)
		if !ok || to != name {
			continue
		}
		if !found || entry.when > best.when {
			best = entry
			found = true
		}
	}
	if !found {
		return ""
	}

	from, _, _ := checkoutEntry(best.what)
	return from
}

// checkoutEntry is the branch a checkout moved from and to.
func checkoutEntry(what string) (string, string, bool) {
	const marker = "checkout: moving from "
	if !strings.HasPrefix(what, marker) {
		return "", "", false
	}
	from, to, ok := strings.Cut(strings.TrimPrefix(what, marker), " to ")
	if !ok {
		return "", "", false
	}
	return from, to, true
}

// reflogEntry is one line of a reflog.
type reflogEntry struct {
	when int64
	what string
}

// readReflog is the reflog of a ref, newest first.
func readReflog(root string, ref string) []reflogEntry {
	stdout, _, code := runGit(
		[]string{"reflog", "show", "--date=unix", "--format=%gd%x09%gs", ref},
		root,
	)
	if code != 0 {
		return nil
	}

	var entries []reflogEntry
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		when, what, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		entries = append(entries, reflogEntry{when: reflogTime(when), what: what})
	}
	return entries
}

// reflogTime is the time in a reflog selector such as "main@{1784520000}".
func reflogTime(selector string) int64 {
	open := strings.Index(selector, "@{")
	if open < 0 {
		return 0
	}
	rest := selector[open+2:]
	end := strings.IndexByte(rest, '}')
	if end < 0 {
		return 0
	}
	when, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil {
		return 0
	}
	return when
}

// remoteBranches is the set of branches that are on the remote.
//
// It is read from the remote tracking refs rather than from the branch
// list, because a branch that is both local and on the remote is listed
// as a local branch, and this needs the remote copy as well.
func remoteBranches(root string) map[string]bool {
	remotes := map[string]bool{}

	stdout, _, code := runGit(
		[]string{"for-each-ref", "--format=%(refname:short)", "refs/remotes/" + defaultRemote},
		root,
	)
	if code != 0 {
		return remotes
	}

	prefix := defaultRemote + "/"
	for _, line := range strings.Split(stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		name = strings.TrimPrefix(name, prefix)
		if name == "" || name == "HEAD" {
			continue
		}
		remotes[name] = true
	}
	return remotes
}
