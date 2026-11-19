## git-picker v1.5

Failures are now reported in full, and the exit code says what went wrong.

### A failure ends the picker

When a git command you asked for fails, the picker stops and shows the error
in full:

```
! repo  git pull failed: fatal: Merge aborted

+-- error --------------------------------
| stdout:
| Auto-merging file
| CONFLICT (content): Merge conflict in file
| stderr:
| error: Automatic merge failed
+---------------------------------------

press any key to exit
```

Any key leaves. The same text is written to the terminal afterwards, so
nothing is lost either way.

Both streams are kept. Git splits the interesting part across them: a merge
conflict writes the conflict lines to stdout and the failure to stderr, so
either one on its own misses half of it.

### The exit code is git's

A failure exits with the exit code git gave, so a script can tell a failed
pull from a failed push:

```
git-picker; echo $?
```

| Exit code | What it means |
| --- | --- |
| 0 | the picker was used and left normally |
| 1 | the picker could not work, or the folder is not a repository |
| 128 | git failed, for example a merge conflict or a remote that is gone |
| other | whatever git exited with |

### A failed fetch does not end the picker

The background fetch fails whenever the machine is offline, which is not
worth closing the app over. It stays on the status line, and the picker
carries on.

A switch, a pull, a commit, a push or the file listing all end the picker
when they fail, because those are the things you asked for.

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
