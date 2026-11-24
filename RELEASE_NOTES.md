## git-picker v1.11

The branch you are on is now at the top of the list, and the text panels
scroll properly.

### The branch you are on is at the top

The branch that is checked out is now the first row of the list, whatever
the sort says and whatever you have typed to filter it. It is the branch
you are most likely to want to see, and before it was buried somewhere in
the middle whenever it was not one of the pinned names.

The pointer starts on it, so the picker opens on the branch you are on
rather than on the top of the sort.

The pinned branches (main, master, develop and the rest) are still pinned
at the top of everything else. They are only pushed down by the branch you
are actually on.

A filter that does not match the branch you are on still hides it. The
filter is you saying that you do not want to see it.

### The text panels scroll

Page up and page down now work in the panels that show text. The bug was
that the panels were listening for a key name that the console never
sends, so the page down key did nothing at all.

The panels that scroll are the git output (ctrl+o), the help panel (the
question mark) and the error panel.

The scrolling also stops at the end of the text. Before, scrolling past the
end left the panel empty, which looked like the picker had stopped
working. The position is now shown at the bottom of the panel, and it says
when you are at the end of the text.

### The error panel scrolls too

The error panel used to cut the error off and say how many lines were left
out. It now scrolls, so the whole error is there to be read.

### Keys

| Key | What it does |
| --- | --- |
| type | filter the list |
| up / down | move the pointer |
| pgup / pgdn | move a screenful, in the list and in the text panels |
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
