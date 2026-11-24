package main

// Updating the picker itself.
//
// The builds are published on the GitHub releases page, so the picker can ask
// which one is the newest and put it in place of the one that is running.
//
// Windows will not let a running program be overwritten, but it will let it
// be renamed, so the old build is moved aside and the new one takes its
// name. The old build is cleared away the next time the picker starts, when
// nothing is holding it open any more.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// updateOwner and updateRepo are where the builds are published.
	updateOwner = "ScottYates"
	updateRepo  = "GitPicker"

	// updateAPI is where the newest release is described.
	updateAPI = "https://api.github.com/repos/" + updateOwner + "/" + updateRepo + "/releases/latest"

	// updateAsset is the name of the build in a release.
	updateAsset = "git-picker.exe"

	// updateOld is the name the build that was replaced is kept under
	// until the picker can clear it away.
	updateOld = ".old"
)

// release is what the GitHub API says about the newest release.
type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// releaseAsset is one file in a release.
type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// asset finds a file in the release.
func (r *release) asset(name string) (releaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

// updateDoneMsg reports what the update did.
type updateDoneMsg struct {
	// version is the version that is now in place.
	version string

	// updated is true when the build was put in place.
	updated bool

	err error
}

// updateCmd fetches the newest build and puts it in place.
//
// The work is done away from the keyboard, so the picker keeps drawing while
// the download runs.
func updateCmd(current string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		client := &http.Client{Timeout: 60 * time.Second}

		rel, err := latestRelease(ctx, client, updateAPI)
		if err != nil {
			return updateDoneMsg{err: err}
		}
		if !newerVersion(rel.TagName, current) {
			return updateDoneMsg{version: rel.TagName}
		}

		asset, ok := rel.asset(updateAsset)
		if !ok {
			return updateDoneMsg{err: fmt.Errorf("the release %s has no %s in it", rel.TagName, updateAsset)}
		}

		data, err := download(ctx, client, asset.BrowserDownloadURL)
		if err != nil {
			return updateDoneMsg{err: err}
		}

		path, err := os.Executable()
		if err != nil {
			return updateDoneMsg{err: err}
		}
		if err := installUpdate(path, data); err != nil {
			return updateDoneMsg{err: err}
		}
		return updateDoneMsg{version: rel.TagName, updated: true}
	}
}

// latestRelease asks GitHub which release is the newest.
func latestRelease(ctx context.Context, client *http.Client, apiURL string) (*release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	// GitHub turns away a request that does not say who it is.
	req.Header.Set("User-Agent", "git-picker/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the release page answered %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var rel release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("the release page did not name a release")
	}
	return &rel, nil
}

// download fetches a file.
func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "git-picker/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the download answered %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// parseVersion reads a tag like v1.10 as the two numbers it is made of.
//
// The tag is the version with a v in front of it, and the numbers are the
// major and the minor one.
func parseVersion(tag string) (int, int, bool) {
	text := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if text == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(text, ".", 3)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor := 0
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return major, minor, true
}

// newerVersion reports whether the tag is a newer build than the version
// that is running.
func newerVersion(tag string, current string) bool {
	tagMajor, tagMinor, ok := parseVersion(tag)
	if !ok {
		return false
	}
	curMajor, curMinor, ok := parseVersion(current)
	if !ok {
		return false
	}
	if tagMajor != curMajor {
		return tagMajor > curMajor
	}
	return tagMinor > curMinor
}

// installUpdate puts the new build in place of the running one.
//
// The new build is written beside the running one first, so the two are on
// the same disk and the swap is a rename. The running one is moved aside
// rather than removed, because Windows will not remove a program that is
// running.
func installUpdate(path string, data []byte) error {
	newPath := path + ".new"
	oldPath := path + updateOld

	if err := os.WriteFile(newPath, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(path, oldPath); err != nil {
		os.Remove(newPath)
		return err
	}
	if err := os.Rename(newPath, path); err != nil {
		// The old build goes back, so the picker still runs.
		os.Rename(oldPath, path)
		os.Remove(newPath)
		return err
	}
	return nil
}

// removeOldBuild clears away the build that was replaced.
//
// It is called when the picker starts, by which time the build that was
// running has finished with its file.
func removeOldBuild(path string) {
	os.Remove(path + updateOld)
}

// relaunch starts the build that was just put in place and waits for it.
//
// The console belongs to one program at a time, so the picker has left it
// before the new build takes it over. The new build is started in the
// folder the picker was working in, and its exit code is the picker's.
func relaunch(path string) (int, error) {
	// A key that is let go of a moment after it was pressed leaves a
	// record behind in the console, and the new build would read it as a
	// key press of its own.
	flushConsoleInput()

	dir, err := os.Getwd()
	if err != nil {
		dir = filepath.Dir(path)
	}

	cmd := exec.Command(path)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// handleUpdateConfirmKey acts on a key press on the question about starting
// the new build.
//
// Saying yes leaves the picker, which has put the console back as it found
// it, and the new build takes the console over. Saying no leaves the picker
// open with the new build waiting for the next time it is started.
func (m model) handleUpdateConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(msg) {
	case "y":
		path, err := os.Executable()
		if err != nil {
			m.phase = phaseList
			m.status = "the update went in, but it could not be started: " + firstLine(err.Error())
			m.statusErr = true
			return m, nil
		}
		m.relaunch = path
		return m, tea.Quit
	case "n", "esc", "enter":
		m.phase = phaseList
		m.status = "updated to " + m.updatedTo + ", start the picker again to use it"
		m.statusErr = false
		return m, nil
	}
	return m, nil
}

// viewUpdateConfirm asks whether the new build should be started.
func (m model) viewUpdateConfirm() string {
	var b strings.Builder

	b.WriteString("  " + "updated" + "\n\n")

	b.WriteString(fmt.Sprintf("%s is in place.\n", m.updatedTo))
	b.WriteString(fmt.Sprintf("the running copy is still %s.\n", version))
	b.WriteString("\n")

	yes := "  start it now"
	no := "  not now"
	if m.updateChoice == 0 {
		yes = pointer + " start it now"
	} else {
		no = pointer + " not now"
	}
	b.WriteString(yes + "\n")
	b.WriteString(no + "\n")

	b.WriteString(styleInfo.Render("y or n, enter to choose, esc to skip"))
	return b.String()
}
