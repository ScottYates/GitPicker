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
//
// Message is the one line summary, and Detail is everything git said. Both
// streams are kept, because git splits the interesting part across them: a
// merge conflict writes the conflict lines to stdout and the failure itself
// to stderr, so either one on its own misses half the story.
type GitError struct {
	Code    int
	Message string
	Detail  string
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
//
// The summary is the first line of what git said, so it fits on the status
// line, and the detail keeps every line of both streams so a failure can be
// reported in full.
func checkGit(stdout, stderr string, code int, action string) (string, error) {
	if code == 0 {
		return stdout, nil
	}

	summary := strings.TrimSpace(stderr)
	if summary == "" {
		summary = strings.TrimSpace(stdout)
	}
	message := fmt.Sprintf("git %s failed", action)
	if summary != "" {
		message += ": " + firstLine(summary)
	}

	return "", &GitError{
		Code:    code,
		Message: message,
		Detail:  gitDetail(stdout, stderr),
	}
}

// gitDetail is everything a failed git command said, labelled by the stream
// it came from.
//
// A stream that said nothing is left out, so the detail reads as git wrote
// it rather than as two empty sections.
func gitDetail(stdout, stderr string) string {
	return gitOutput(stdout, stderr)
}

// gitOutput is everything a git command said, labelled by the stream it
// came from.
//
// A stream that said nothing is left out, so the output reads as git wrote
// it rather than as two empty sections.
func gitOutput(stdout, stderr string) string {
	var parts []string
	if out := strings.TrimSpace(stdout); out != "" {
		parts = append(parts, "stdout:\n"+out)
	}
	if err := strings.TrimSpace(stderr); err != "" {
		parts = append(parts, "stderr:\n"+err)
	}
	return strings.Join(parts, "\n\n")
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
//
// The output comes back as well, so that a failure can be reported in full.
func fetch(cwd string) (string, error) {
	stdout, stderr, code := runGit([]string{"fetch", "--all", "--prune"}, cwd)
	_, err := checkGit(stdout, stderr, code, "fetch")
	return gitOutput(stdout, stderr), err
}

// switchBranch checks out a local branch, or tracks a remote-only one.
func switchBranch(branch Branch, cwd string) (string, error) {
	args := switchArgs(branch)
	stdout, stderr, code := runGit(args, cwd)
	_, err := checkGit(stdout, stderr, code, strings.Join(args, " "))
	return gitOutput(stdout, stderr), err
}

// pull updates the current branch from its upstream.
func pull(cwd string) (string, error) {
	stdout, stderr, code := runGit([]string{"pull"}, cwd)
	_, err := checkGit(stdout, stderr, code, "pull")
	return gitOutput(stdout, stderr), err
}

// createBranch makes a new branch at the current HEAD and switches to it.
func createBranch(name string, cwd string) (string, error) {
	stdout, stderr, code := runGit([]string{"switch", "-c", name}, cwd)
	_, err := checkGit(stdout, stderr, code, "switch -c "+name)
	return gitOutput(stdout, stderr), err
}

// FileStatus is one entry from git status.
//
// The code is git's own short form, such as " M" for a change in the
// working tree, "M " for one in the index and "??" for an untracked file.
type FileStatus struct {
	Code string
	Path string
}

// statusFiles lists the files git has something to say about.
//
// This is the short format, one entry per line: two characters of status, a
// space, then the path. A rename shows as "old -> new" and the new path is
// the one that gets committed.
//
// The output comes back as well, though the file list is already that
// output shown as a list.
func statusFiles(cwd string) ([]FileStatus, string, error) {
	stdout, stderr, code := runGit([]string{"status", "--porcelain"}, cwd)
	if _, err := checkGit(stdout, stderr, code, "status"); err != nil {
		return nil, gitOutput(stdout, stderr), err
	}

	var files []FileStatus
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) < 4 {
			continue
		}
		path := line[3:]
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		files = append(files, FileStatus{Code: line[:2], Path: path})
	}
	return files, gitOutput(stdout, stderr), nil
}

// commitFiles commits the given paths with the message.
//
// The files are added first, because a path-limited commit cannot pick up a
// file git has never seen. The commit is then limited to those same paths,
// so anything else that happened to be staged is left alone.
func commitFiles(cwd, message string, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("no files were selected")
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("a commit message is needed")
	}

	add := append([]string{"add", "--"}, paths...)
	stdout, stderr, code := runGit(add, cwd)
	if _, err := checkGit(stdout, stderr, code, "add"); err != nil {
		return gitOutput(stdout, stderr), err
	}

	commit := append([]string{"commit", "-m", message, "--"}, paths...)
	stdout, stderr, code = runGit(commit, cwd)
	_, err := checkGit(stdout, stderr, code, "commit")
	return gitOutput(stdout, stderr), err
}

// push pushes the current branch to its upstream.
//
// The output comes back as well, because git prints the address of the
// pull request at the end of a push, and that is the address the browser
// should open.
func push(cwd string) (string, error) {
	stdout, stderr, code := runGit([]string{"push"}, cwd)
	_, err := checkGit(stdout, stderr, code, "push")
	return gitOutput(stdout, stderr), err
}

// cloneRepo clones a repository into a directory.
//
// The output comes back as well, because a clone says a good deal about
// what it is doing.
func cloneRepo(address, cwd string) (string, error) {
	stdout, stderr, code := runGit([]string{"clone", address}, cwd)
	_, err := checkGit(stdout, stderr, code, "clone "+address)
	return gitOutput(stdout, stderr), err
}
