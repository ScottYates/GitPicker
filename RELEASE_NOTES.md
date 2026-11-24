## git-picker v1.10

The picker can update itself, the keys it understands are written down in a
panel, and the hint at the bottom no longer changes before it has to.

### The picker can update itself

Ctrl+U asks GitHub which build is the newest. If it is newer than the one
that is running, it is fetched and put in place of the running one. The
picker then asks whether the new build should be started, and hands the
console over to it.

Windows will not let a running program be overwritten, so the old build is
renamed out of the way and the new one takes its name. The old build is
cleared away the next time the picker starts, when nothing is holding it
open any more.

The build that is fetched is the one published on the release page, so the
picker and the releases stay in step. When there is nothing newer the status
line says so and nothing is written to disk. When the picker is running a
build that is newer than the published one, it says that too.

The update needs the network, and it fails quietly enough: the reason is
shown in the status line and the picker carries on.

### The keys are written down

The question mark opens a panel with every key there is and what it does.
The panel scrolls when there is more of it than the screen can take.

The question mark is only a key in the branch list. In the commit message
it is ordinary text, as it always was.

### The hint at the bottom stays put

The hint used to change to a shorter form as soon as the window was a little
narrower than the whole of it, which was a long way before there was any
need for it. It is now written on two lines, and only changes when the
window is too narrow for both of them.

The hint in the commit message box does the same.

### What git said can be shown again

Ctrl+O shows what git last said, or hides it while it is on screen. The text
is kept after the panel has been closed, so it can be read again without
running the command once more. The output still opens by itself after every
git command, as it did before.

### The pointer is only for the picked row

The arrow was in front of the header and in front of the search box as well
as in front of the picked branch, so the same mark meant three different
things. It is now only in front of the row that is picked, and in front of
the choice that is pointed at when there is a question to answer.

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
| ctrl+o | show or hide what git last said |
| ctrl+u | update to the newest build |
| ? | the panel with every key in it |
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
