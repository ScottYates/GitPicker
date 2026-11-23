package main

// git-picker: an interactive git branch picker that switches and pulls.
//
// Version: v1.7
//
// Finds the repository root, lists local and remote branches in a
// fuzzy-selectable menu, switches to the chosen branch and pulls.
//
// The fetch runs in the background, so the list is on screen without waiting
// for the network, and it refreshes when the fetch lands.
//
// The picker fills the whole screen, reflows when the window is resized,
// pages with PageUp/PageDown, shows its position in the list, shows the
// repository folder name at the top, and displays an ASCII panel with the
// exact commands it will run. Ctrl+N asks for a name and makes a new branch
// from the highlighted one.
//
// Switching does not close the picker: the branch is switched, the list is
// rebuilt and the picker carries on, so branches can be worked through one
// after another. Escape leaves.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "v1.7"

func main() {
	os.Exit(mainExitCode(os.Args[1:], os.Stdout, os.Stderr))
}

// mainExitCode runs the picker and returns the exit code.
//
// A git failure exits with the exit code git gave, so a script can tell a
// failed pull from a failed push instead of seeing a plain success.
// Anything else that goes wrong exits 1. Leaving the picker with escape is a
// normal ending, so it exits 0.
func mainExitCode(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("git-picker", flag.ContinueOnError)
	flags.SetOutput(stderr)
	showVersion := flags.Bool("version", false, "print the version and exit")
	dump := flags.Bool("dump-labels", false, "print the branch list as the picker draws it and exit")
	query := flags.String("query", "", "filter the dumped list with this search")
	now := flags.Int64("now", 0, "override the clock used for the age column")

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, "git-picker "+version)
		return 0
	}

	if err := run(*dump, *now, *query, stdout); err != nil {
		return reportError(err, stderr)
	}
	return 0
}

// reportError writes the error out in full and returns the exit code.
//
// A git failure carries the exit code git gave, and that is what the process
// ends with. The detail is written out as well, because the summary is only
// the first line of what git said.
func reportError(err error, stderr io.Writer) int {
	gitErr, ok := err.(*GitError)
	if !ok {
		fmt.Fprintf(stderr, "Error: %s\n", err)
		return 1
	}

	fmt.Fprintf(stderr, "Error: %s\n", gitErr.Message)
	if gitErr.Detail != "" {
		fmt.Fprintln(stderr, gitErr.Detail)
	}

	code := gitErr.Code
	if code == 0 {
		code = 1
	}
	return code
}

// run does the work and returns an error, if any.
func run(dump bool, now int64, query string, stdout io.Writer) error {
	root, ok := findRepoRoot(workingDir())
	if !ok {
		// There is no repository, but there may be one on the clipboard
		// to clone, so the picker starts anyway and waits to hear about
		// it. Leaving without a repository is a failure, because there
		// was nothing to work on.
		return runWithoutRepo()
	}

	if dump {
		return dumpLabels(root, now, query, stdout)
	}

	// The fetch runs in the background from the very start, so it overlaps
	// the listing below and is already part way through by the time the
	// picker opens.
	fetchCh := make(chan fetchResult, 1)
	go func() {
		_, err := fetch(root)
		var branches []Branch
		if err == nil {
			branches, _ = listBranches(root)
		}
		fetchCh <- fetchResult{branches: branches, err: err}
	}()

	branches, err := listBranches(root)
	if err != nil {
		return err
	}
	if len(branches) == 0 {
		return fmt.Errorf("no branches found")
	}

	current := currentBranch(root)
	picker := newModel(root, repoLabel(root), current, branches, fetchCh, loadHistory())

	return showPicker(picker)
}

// runWithoutRepo runs the picker where there is no repository.
//
// The picker starts empty, with a clone offered if the clipboard holds the
// address of one. If there is nothing to clone, leaving is a failure, so
// the picker says what is wrong and exits with the code for it.
func runWithoutRepo() error {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	name := filepath.Base(dir)

	picker := newModel("", name, "", nil, nil, loadHistory())
	picker.cloneDir = dir
	picker.status = "not inside a git repository"
	picker.statusErr = true

	if err := showPicker(picker); err != nil {
		return err
	}
	return noRepositoryError()
}

// noRepositoryError is what the picker reports when it is left with no
// repository to work on.
//
// It is a failure rather than a quiet exit, because there was nothing the
// picker could do.
func noRepositoryError() error {
	return fmt.Errorf("not inside a git repository")
}

// showPicker runs the picker and reports how it ended.
func showPicker(picker model) error {
	program := tea.NewProgram(picker, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return err
	}

	m, ok := final.(model)
	if !ok {
		return fmt.Errorf("the picker ended unexpectedly")
	}
	// A git failure the user asked for ends the picker, and the error
	// carries the exit code git gave.
	if m.fatal != nil {
		return m.fatal
	}
	// Escape leaves the picker, which is a normal ending rather than a
	// cancellation, so it exits quietly.
	return nil
}

// workingDir is the directory the picker was started in.
func workingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// dumpLabels prints the branch list exactly as the picker would draw it.
//
// This is what the parity check compares against the Python picker, so the
// two can be held to the same output without either needing a terminal. With
// a query it prints the fuzzy matches in rank order instead.
func dumpLabels(root string, now int64, query string, stdout io.Writer) error {
	branches, err := listBranches(root)
	if err != nil {
		return err
	}
	sortBranches(branches, sortTime)
	current := currentBranch(root)
	width := labelNameWidth(branches)
	labels := make([]string, len(branches))
	for i, branch := range branches {
		labels[i] = displayLabel(branch, current, now, width)
	}

	if query == "" {
		for _, label := range labels {
			fmt.Fprintln(stdout, label)
		}
		return nil
	}
	for _, entry := range filterBranches(query, branches, labels) {
		fmt.Fprintln(stdout, labels[entry.index])
	}
	return nil
}
