## git-picker v1.9

The mouse can select the text on screen, and the characters the picker draws
are drawn properly.

### The mouse can select the text

The picker used to turn the console's quick edit mode off, which is what a
program that reads the keyboard has to do, and the mouse could not select
anything in the window afterwards. Now the quick edit mode is on, so the
text the picker shows, such as what git said or the error it hit, can be
selected with the mouse and copied.

The price is the one the classic console charges for it: a click in the
window pastes the clipboard into the picker, so a click that was meant to
be a click types whatever is on the clipboard. A drag to select does not
do that, and the error panel now waits for escape instead of leaving on any
key, so a stray click cannot close it while the error is being read.

### The characters are drawn properly

The console reads what a program writes with the code page it is on, and
that is usually one of the DOS code pages, where 437 is the common one. The
bytes of a character outside ASCII were then read as two or three
characters of that page, so the pointer in front of the selected branch
came out as three pieces of rubbish.

The picker now puts the console on UTF-8 while it runs, and puts it back
when it is done, so the console reads the characters as they are.

### The panels are drawn with line drawing characters

```
┌─ will run ──────────────────────────────────────────────────────────────────
│ git switch master                                                           
│ git pull                                                                     
```

The pointer is the arrow of the console's own character set, so it is the
same in every console font.

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
