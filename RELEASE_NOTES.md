## git-picker v1.4

Ask before pushing, so a commit goes out when you want it to.

### The push question

After a successful commit the picker stops and asks:

```
? repo  Push main to origin/main?

> yes
  no

y or n, enter to choose, esc to skip
```

Answer `y` and the commit is pushed. Answer `n`, or press escape, and it is
left where it is. The arrow keys move the pointer and enter takes the
answer, so the question can be answered without the keyboard shortcuts.

The question names the upstream when the branch has one. A branch with no
upstream just says `Push main?`, and the push fails in the status line if
git refuses it.

The picker stays open through all of it. Nothing closes until you press
escape.

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
| y / n | answer the push question after a commit |
| esc | leave |

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
