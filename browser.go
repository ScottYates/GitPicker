package main

// Opening a web page in the default browser.

import (
	"os/exec"
	"runtime"
)

// openBrowser opens an address in the default browser.
//
// It is a variable so a test can stand in for it, rather than opening a
// real browser while the tests run.
var openBrowser = defaultOpenBrowser

// defaultOpenBrowser opens an address the way each platform does.
//
// The address is handed over as a separate argument rather than through a
// shell, so a branch name with a space in it cannot break it.
func defaultOpenBrowser(address string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address).Run()
	case "darwin":
		return exec.Command("open", address).Run()
	default:
		return exec.Command("xdg-open", address).Run()
	}
}
