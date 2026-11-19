package main

// git-picker: an interactive git branch picker that switches and pulls.
//
// Version: v1.4
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
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "v1.4"

// exitCancelled is the exit code for a cancelled picker.
const exitCancelled = 130

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	dump := flag.Bool("dump-labels", false, "print the branch list as the picker draws it and exit")
	query := flag.String("query", "", "filter the dumped list with this search")
	now := flag.Int64("now", 0, "override the clock used for the age column")
	flag.Parse()

	if *showVersion {
		fmt.Println("git-picker " + version)
		return
	}

	if err := run(*dump, *now, *query); err != nil {
		if gitErr, ok := err.(*GitError); ok {
			fmt.Fprintf(os.Stderr, "Error: %s\n", gitErr.Message)
			os.Exit(gitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// run does the work and returns an error, if any.
func run(dump bool, now int64, query string) error {
	root, ok := findRepoRoot(workingDir())
	if !ok {
		return fmt.Errorf("not inside a git repository")
	}

	if dump {
		return dumpLabels(root, now, query)
	}

	// The fetch runs in the background from the very start, so it overlaps
	// the listing below and is already part way through by the time the
	// picker opens.
	fetchCh := make(chan fetchResult, 1)
	go func() {
		err := fetch(root)
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

	program := tea.NewProgram(picker, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return err
	}

	if _, ok := final.(model); !ok {
		return fmt.Errorf("the picker ended unexpectedly")
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
func dumpLabels(root string, now int64, query string) error {
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
			fmt.Println(label)
		}
		return nil
	}
	for _, entry := range filterBranches(query, branches, labels) {
		fmt.Println(labels[entry.index])
	}
	return nil
}
