package main

// Cloning a repository.
//
// The address comes either from the clipboard or from the user typing it,
// and the clone goes into the current directory as a folder named after
// the repository.

import (
	"path/filepath"
	"strings"
)

// gitHosts are the hosts a repository address is known to live on.
//
// An address on one of these is taken to be a repository even when it does
// not end in .git, because that is how the address is usually copied.
var gitHosts = map[string]bool{
	"github.com":        true,
	"gitlab.com":        true,
	"bitbucket.org":     true,
	"codeberg.org":      true,
	"gitea.com":         true,
	"git.sr.ht":         true,
	"dev.azure.com":     true,
	"ssh.dev.azure.com": true,
}

// gitAddress is a repository address found in some text.
//
// It is the first line that looks like a repository, because a copy of an
// address is often a line in the middle of something else.
func gitAddress(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isGitAddress(line) {
			return line
		}
	}
	return ""
}

// isGitAddress reports whether a line is the address of a repository.
//
// The forms are the one git uses itself, and the ones a browser would
// produce:
//
//	git@github.com:owner/repo.git
//	ssh://git@github.com/owner/repo.git
//	https://github.com/owner/repo.git
//	https://github.com/owner/repo
func isGitAddress(line string) bool {
	if strings.HasPrefix(line, "git@") {
		// The scp form, git@host:owner/repo.
		host, rest, ok := strings.Cut(strings.TrimPrefix(line, "git@"), ":")
		return ok && rest != "" && !strings.ContainsAny(host, "/\\ ")
	}

	for _, prefix := range []string{"https://", "http://", "ssh://", "git://"} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimPrefix(line, prefix)
		host, path, ok := strings.Cut(rest, "/")
		if !ok || path == "" {
			return false
		}
		if gitHosts[host] {
			return true
		}
		return strings.HasSuffix(strings.TrimSuffix(path, "/"), ".git")
	}

	return false
}

// repoFolder is the name of the folder a clone of an address goes into.
//
// It is the last part of the address, without the .git and without any
// trailing slash, which is what git itself uses. A path on disk is
// allowed as well, and a path on Windows is separated by backslashes.
func repoFolder(address string) string {
	name := strings.TrimSuffix(strings.TrimSpace(address), "/")
	if i := strings.LastIndexAny(name, "/:\\"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	return name
}

// cloneTarget is the folder a clone of an address goes into, which is a
// folder of that name in the given directory.
//
// The path is checked to be inside the directory, because the folder is
// removed before a fresh clone and it must be the one git would create.
func cloneTarget(address, dir string) (string, bool) {
	name := repoFolder(address)
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	target := filepath.Join(dir, name)
	if filepath.Dir(target) != filepath.Clean(dir) {
		return "", false
	}
	return target, true
}
