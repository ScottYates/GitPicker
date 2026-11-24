# git-picker, a handoff

This is what you need to work on git-picker without reading the whole source from the top.

## What it is

A single file Windows program that picks a git branch. It finds the
repository, lists the local and remote branches in a fuzzy searchable menu,
and switches to the one you pick and pulls it. It also commits, pushes, and
clones.

It is written in Go with [bubbletea](https://github.com/charmbracelet/bubbletea)
for the terminal interface and [lipgloss](https://github.com/charmbracelet/lipgloss)
for the drawing.

The repository also holds `git_picker.py`, the Python program this was ported
from. It is kept as the frozen reference: the Go program is the one that
ships, and `parity_check.py` compares the two row for row so the port cannot
quietly drift. It compares the branch list and the fuzzy ranking only, so the
Go side is free to draw whatever it likes.

## The shape of the source

| File | What is in it |
| --- | --- |
| `main.go` | the flags, the start up, and the version |
| `ui.go` | the model, the key handling and every view |
| `git.go` | every git command, and the error they give |
| `branch.go` | the branch list, and how it is labelled |
| `fuzzy.go` | the fuzzy matching and its ranking |
| `message.go` | the commit message as text, and where the caret is |
| `input.go` | the seam between the picker and the keyboard |
| `input_windows.go` | the console reader, and the key map it uses |
| `input_other.go` | the same, for the builds that let bubbletea read |
| `history.go` | the saved commit messages |
| `clone.go`, `pullrequest.go`, `browser.go` | the other jobs the picker does |
| `clipboard_windows.go`, `clipboard_other.go` | the clipboard, per platform |
| `paths.go`, `longpath_windows.go`, `longpath_other.go` | where things live on disk |

## Building and running it

```
.\build.ps1
```

That leaves `git-picker.exe` in the folder: one file with no runtime to
install, which is why it starts in tens of milliseconds. `git-picker.cmd` is
a launcher for it, so the folder can be put on the PATH and the picker can be
started by name.

The program has to be started inside a repository, or in a folder where
something can be cloned into.

## The tests

```
go test ./...
python parity_check.py
```

The Go tests cover the model, the drawing and the git commands. The parity
check runs the Python program and the Go program over the same repositories
and compares what they print.

Two of the tests are worth knowing about:

- `input_console_test.go` reads the real console. It writes a key into the
  console of whoever is running it, so it only runs when it is asked for:

  ```
  $env:GITPICKER_CONSOLE_TEST='1'
  go test -run TestTheConsoleReader -v .
  ```

  It skips itself where there is no console, which is every CI runner.

- `render_snapshot_test.go` prints the picker as it is drawn, so the layout
  can be looked at:

  ```
  go test -run TestRenderSnapshot -v
  ```

## Releasing

1. Put the new version in two places: the `// Version:` line at the top of
   `main.go` and the `version` constant below it. They have to agree.
2. Write `RELEASE_NOTES.md`. The release workflow uses it as the body of the
   release, so it has to describe that release and not the last one.
3. Commit, push, tag and push the tag:

   ```
   git commit -am "the change"
   git push origin main
   git tag -a v1.8 -m "git-picker v1.8"
   git push origin v1.8
   ```

4. The tag pushes the workflow, which builds, tests and publishes the release
   with `git-picker.exe` and `HANDOFF.md` attached.
5. Check it:

   ```
   gh release view v1.8 --json assets
   gh release download v1.8 --dir <somewhere> --pattern "git-picker.exe" --clobber
   ```

   Run the downloaded file. A release that was not run is not a release.

A failed release workflow publishes nothing, so a tag can be moved and pushed
again if the tests fail.

## The things that are easy to get wrong

These are the traps in this program. Each one cost time to find.

**Bubbletea cannot tell ctrl+enter from enter on Windows.** Its key table
maps the return key to enter whatever else is held down, so the ctrl bit is
gone before the model ever sees the key. The console record still carries
it, which is why `input_windows.go` reads the records itself and hands the
keys over with `program.Send`. The console reader maps ctrl+enter onto the
ctrl+j key, because a terminal sends the same byte for the two and that is
the name both arrive under.

**The console reader owns the input.** Bubbletea is told there is no input at
all (`tea.WithInput(nil)`), so the picker is the only thing reading the
console. Two readers on one console would each steal the other's keys.

**The virtual terminal input has to stay off.** With
`ENABLE_VIRTUAL_TERMINAL_INPUT` the console throws the modifier keys away
and with them the difference between enter and ctrl+enter. The mode is set
in `prepareConsole` and the mode that was there before is put back when the
picker is done.

**The console is the only source of window resizes on Windows.** Bubbletea's
own resize listener does nothing there, so the reader has to forward the
window size records or the picker stops reflowing.

**`tea.KeyCtrlM` and `tea.KeyEnter` are the same value.** They cannot both be
listed in one switch, and a key type that is not in bubbletea's table prints
as an empty string.

**The space bar is not a rune.** Bubbletea turns a single space into its own
key type, so a handler that only looks at `msg.Runes` drops every space in a
commit message. That is what `isSpaceKey` is for.

**The terminal cursor is hidden.** Bubbletea hides it for the whole run of the
program, so a text field has to draw its own caret, which is what
`withCursor` does.

**Git words the same failure differently on different machines.** A test that
asserts on the text git printed will pass on one machine and fail on another.
Assert on the exit code, and on your own summary line.

**One git command at a time.** Git will not let two commands touch the index
at once, so an action that arrives while another one is running waits its
turn in `pendingAction` rather than racing it.

**The exit code is git's.** A git failure the user asked for ends the picker
with the code git gave, so a script can tell a failed pull from a failed
push. A background fetch never does.

## Where it could go next

- The list shows one line per branch. A branch with a lot of history could do
  with more.
- There is no search across the commit history. The saved messages are the
  only memory of what has been written.
- The picker only reads the clipboard when there is no repository. Reading it
  in a repository as well would need a way to say what it is for.
- The console reader is Windows only. On the other platforms ctrl+enter
  arrives as ctrl+j and works, but the resize handling is bubbletea's.
