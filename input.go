package main

// The keyboard the picker reads.
//
// On Windows the picker reads the console records itself, because bubbletea
// cannot tell ctrl+enter from enter. Its key table maps the return key to
// enter no matter what else is held down, so the ctrl bit is thrown away
// before the picker ever sees it. The console record still carries it, so
// the picker reads the console and hands the keys over itself.
//
// Everywhere else bubbletea reads the keyboard and the picker has nothing to
// do.

import tea "github.com/charmbracelet/bubbletea"

// keyboard is where the key presses come from.
//
// An empty keyboard is one that has nothing to do, which is what a build
// that lets bubbletea read the keyboard has.
type keyboard struct {
	// options are the program options the keyboard needs.
	options []tea.ProgramOption

	// start begins handing key presses to the program. It returns the
	// function that stops the reader again.
	start func(p *tea.Program) func()
}

// begin starts the keyboard and returns the function that stops it.
//
// A keyboard that reads nothing has nothing to start.
func (k keyboard) begin(p *tea.Program) func() {
	if k.start == nil {
		return func() {}
	}
	return k.start(p)
}
