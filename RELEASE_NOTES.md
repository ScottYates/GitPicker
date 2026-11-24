## git-picker v1.8

A commit message of more than one line, and a spinner while git works.

### The commit message can be more than one line

Enter adds a line to the message now, and ctrl+enter is what finishes it:

```
? repo  Commit message:
> fix the thing that was broken
> and the other thing as well
ctrl+enter to commit, enter for a new line, esc to go back
```

The up and down keys move the caret between the lines, so a message of more
than one line can be worked through, and backspace at the start of a line
joins it to the line above.

The saved messages are walked with ctrl+p and ctrl+n, because the up and down
keys belong to the caret now:

| Key | What it does |
| --- | --- |
| ctrl+p | the older saved message |
| ctrl+n | the newer saved message, and then an empty message |

The message is handed to git as it is, so both lines are in the commit.

### A spinner while git works

The status line carries a spinner while a git command is running, so a slow
pull does not look like a hang:

```
? repo  Switch to branch:
>   [1/4]  sort: time  | switching to master...
```

The spinner goes round while the work lasts and stops by itself when the work
is done.

### The keyboard is read from the console

On Windows the picker now reads the console records itself instead of letting
bubbletea do it. Bubbletea maps the return key to enter whatever else is held
down, so it cannot tell ctrl+enter from enter, and the ctrl bit is thrown
away before the picker ever sees the key. The console record still carries
it, so the picker reads the records and hands the keys over itself.

Everything else about the keys is unchanged. The keys behave exactly as they
did, the window still reflows when it is resized, and a console that cannot
be opened falls back to the reader that was there before.

### Keys

| Key | What it does |
| --- | --- |
| type | filter the list |
| up / down | move the pointer |
| pgup / pgdn | move a screenful |
| ctrl+s | sort by time or by name, pinned branches first |
| ctrl+n | switch to the highlighted branch and make a new one from it |
| ctrl+c | commit: pick the files, then write the message |
| ctrl+p | push the current branch |
| ctrl+g | clone a repository into the current directory |
| enter | switch to the highlighted branch and pull |
| y / n | answer the push question, and the pull request question |
| esc | leave, or close the git output |

In the commit message box:

| Key | What it does |
| --- | --- |
| ctrl+enter | finish the message and commit |
| enter | add a line to the message |
| up / down | move the caret between the lines |
| ctrl+p / ctrl+n | walk the saved messages |
| esc | go back to the file list |

Escape is the way out. Escape always leaves, even while git is working.

### Install

Download `git-picker.exe` and run it from inside a repository. Put it
somewhere on your PATH if you want to start it by name.

### Building it yourself

```
git clone https://github.com/ScottYates/GitPicker
cd GitPicker
go build -ldflags "-s -w" -o git-picker.exe .
```

The repository also holds `git_picker.py`, the Python implementation this was
ported from. `parity_check.py` compares the two row for row, including the
fuzzy ranking, and it runs in CI on every push.
