## git-picker v1.3

Commit and push from inside the picker.

### Ctrl+C commits

Ctrl+C opens the changed files, you pick the ones you want with space, and
then the commit message is asked for. Only the files you ticked go into the
commit, so anything else that happened to be staged is left alone. The
picker stays open afterwards, and the commit is confirmed in the status
line.

### Ctrl+P pushes

Ctrl+P pushes the current branch to its upstream and reports what happened.

### Commit message history

The last ten commit messages are kept in `.git-picker-commit-history` in your
home folder. Up and down in the message box walk back through them, and a
message you use again moves to the front rather than appearing twice.

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
| enter | switch to the highlighted branch and pull |
| esc | leave |

Ctrl+C is a commit key now, so Escape is the way out. Escape always leaves,
even while git is working.

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
