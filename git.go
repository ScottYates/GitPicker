package main

// The git layer: every git command the picker runs, plus the repo root
// lookup. Output is captured so git never leaks into the menu.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GitError is a git command that failed. It carries git's own exit code so
// the picker can exit with it.
type GitError struct {
	Code    int
	Message string
}

func (e *GitError) Error() string { return e.Message }

// runGit runs git and returns its output.
//
// stdout and stderr are captured separately and git's exit code is returned,
// so a failure can be reported with git's own words.
func runGit(args []string, cwd string) (string, string, int) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			// git is missing or could not be started at all.
			code = 127
		}
	}
	return stdout.String(), stderr.String(), code
}

// checkGit returns stdout, or a GitError carrying git's exit code.
func checkGit(stdout, stderr string, code int, action string) (string, error) {
	if code != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		return "", &GitError{Code: code, Message: fmt.Sprintf("git %s failed: %s", action, detail)}
	}
	return stdout, nil
}

// findRepoRoot walks upward from start until a .git entry is found.
//
// Accepts both a .git directory and the .git file that worktrees and
// submodules use, so linked checkouts resolve correctly too. The start path
// is expanded to its long form first, so the returned root (and the header
// derived from it) never carries an 8.3 alias.
func findRepoRoot(start string) (string, bool) {
	current := longPath(absPath(start))
	for {
		if pathExists(current + `\.git`) {
			return current, true
		}
		parent := dirName(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// branchNameFromRef recovers the true branch name from a full refname.
//
// Deliberately avoids %(refname:short). When a branch and a tag share a
// name, git disambiguates the short form to 'heads/feature/x', which is not
// a branch name you can switch to. Stripping the known prefix ourselves is
// exact regardless of tags or other refs sharing the name.
func branchNameFromRef(ref string) string {
	if strings.HasPrefix(ref, localRefPrefix) {
		return ref[len(localRefPrefix):]
	}
	if strings.HasPrefix(ref, remoteRefPrefix) {
		return ref[len(remoteRefPrefix):]
	}
	return ref
}

// refFormat is the for-each-ref format. The refname is the FULL one on
// purpose; see branchNameFromRef.
const refFormat = "--format=%(refname)\t%(committerdate:unix)\t%(upstream:short)"

// listBranches collects the local branches plus the remote-only branches
// from origin.
//
// One for-each-ref call covers both namespaces. The cost of a git
// subprocess is a few milliseconds, and startup is the whole point of this
// program, so it is worth the single call.
func listBranches(cwd string) ([]Branch, error) {
	stdout, stderr, code := runGit(
		[]string{"for-each-ref", refFormat, "refs/heads", "refs/remotes/" + defaultRemote},
		cwd,
	)
	if _, err := checkGit(stdout, stderr, code, "for-each-ref"); err != nil {
		return nil, err
	}

	type localInfo struct {
		timestamp int64
		upstream  string
	}
	local := map[string]localInfo{}
	remote := map[string]int64{}

	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		ref, timestamp, upstream := parseRefLine(line)
		if ref == "" {
			continue
		}
		switch {
		case strings.HasPrefix(ref, localRefPrefix):
			local[branchNameFromRef(ref)] = localInfo{timestamp: timestamp, upstream: upstream}
		case strings.HasPrefix(ref, remoteRefPrefix):
			if ref == remoteHeadRef {
				continue
			}
			name := branchNameFromRef(ref)
			if name == "" {
				continue
			}
			if _, seen := remote[name]; !seen {
				remote[name] = timestamp
			}
		}
	}

	branches := make([]Branch, 0, len(local)+len(remote))
	for name, info := range local {
		branches = append(branches, Branch{
			Name:      name,
			Remote:    false,
			Timestamp: info.timestamp,
			Upstream:  info.upstream,
		})
	}
	for name, timestamp := range remote {
		if _, exists := local[name]; exists {
			continue
		}
		branches = append(branches, Branch{Name: name, Remote: true, Timestamp: timestamp})
	}
	return branches, nil
}

// parseRefLine pulls the refname, the unix commit date and the upstream out
// of one for-each-ref line.
//
// The upstream is whatever follows the second tab, so a ref name containing
// a tab cannot shift the other fields.
func parseRefLine(line string) (string, int64, string) {
	first := strings.IndexByte(line, '\t')
	if first < 0 {
		return strings.TrimSpace(line), 0, ""
	}
	ref := strings.TrimSpace(line[:first])
	rest := line[first+1:]

	second := strings.IndexByte(rest, '\t')
	if second < 0 {
		return ref, parseTimestamp(rest), ""
	}
	timestamp := parseTimestamp(rest[:second])
	upstream := strings.TrimSpace(rest[second+1:])
	return ref, timestamp, upstream
}

// parseTimestamp reads a unix timestamp, treating anything unparseable as
// zero rather than failing the whole listing.
func parseTimestamp(text string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// currentBranch is the name of the checked out branch, or "" on a detached
// or unborn HEAD.
//
// The ref is read straight out of the git directory rather than by asking
// git, because a git subprocess costs about 25 milliseconds and this sits on
// the startup path. Every failure falls back to asking git directly, so the
// answer is the same either way.
//
// The full ref is used rather than the abbreviated form, because the
// abbreviated form disambiguates against tags: a branch named feature/x
// alongside a tag of the same name would come back as 'heads/feature/x' and
// the current-branch marker would never match.
func currentBranch(cwd string) string {
	if ref := headFromGitDir(cwd); ref != "" {
		return branchNameFromRef(ref)
	}
	stdout, _, code := runGit([]string{"symbolic-ref", "--quiet", "HEAD"}, cwd)
	if code != 0 {
		return ""
	}
	ref := strings.TrimSpace(stdout)
	if ref == "" {
		return ""
	}
	return branchNameFromRef(ref)
}

// headFromGitDir reads HEAD straight out of the git directory.
//
// It handles both a .git directory and the .git file that worktrees and
// submodules use, and it returns "" when HEAD cannot be read or is a
// detached head, in which case git itself has to be asked.
func headFromGitDir(root string) string {
	gitPath := filepath.Join(root, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}

	dir := gitPath
	if !info.IsDir() {
		// A .git file holds "gitdir: <path>".
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
		if !ok {
			return ""
		}
		target := strings.TrimSpace(rest)
		if target == "" {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(root, target)
		}
		dir = target
	}

	data, err := os.ReadFile(filepath.Join(dir, "HEAD"))
	if err != nil {
		return ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "ref:")
	if !ok {
		// A detached head holds a commit id, not a ref.
		return ""
	}
	return strings.TrimSpace(rest)
}

// fetch updates the remote refs and prunes the ones that are gone.
func fetch(cwd string) error {
	stdout, stderr, code := runGit([]string{"fetch", "--all", "--prune"}, cwd)
	_, err := checkGit(stdout, stderr, code, "fetch")
	return err
}

// switchBranch checks out a local branch, or tracks a remote-only one.
func switchBranch(branch Branch, cwd string) error {
	args := switchArgs(branch)
	stdout, stderr, code := runGit(args, cwd)
	_, err := checkGit(stdout, stderr, code, strings.Join(args, " "))
	return err
}

// pull updates the current branch from its upstream.
func pull(cwd string) error {
	stdout, stderr, code := runGit([]string{"pull"}, cwd)
	_, err := checkGit(stdout, stderr, code, "pull")
	return err
}

// createBranch makes a new branch at the current HEAD and switches to it.
func createBranch(name string, cwd string) error {
	stdout, stderr, code := runGit([]string{"switch", "-c", name}, cwd)
	_, err := checkGit(stdout, stderr, code, "switch -c "+name)
	return err
}
