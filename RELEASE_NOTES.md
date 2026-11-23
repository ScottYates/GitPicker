## git-picker v1.7

Clone a repository, and see what git said while it did it.

### Ctrl+G clones

Ctrl+G asks for the address of a repository and clones it into the current
directory, as a folder named after the repository:

```
? folder  Clone:

> https://github.com/owner/repo.git
+-- will run --------------------------------
| git clone https://github.com/owner/repo.git
-------------------------------------------

enter to clone, esc to cancel
```

If a folder of that name is already there, the picker asks before removing
it, because that is the only way to get a fresh copy:

```
? folder  Remove C:\work\repo and clone again?

> yes
  no
```

After the clone the picker moves into the new repository, so the branches in
it are the ones on offer.

### The clipboard is looked at

Started outside a repository, the picker reads the clipboard. If it holds the
address of a repository, the picker offers to clone it:

```
? folder  Clone repo?

> yes
  no

git clone https://github.com/owner/repo.git
```

An address is recognised in the forms git itself uses,
`git@github.com:owner/repo.git` and `https://github.com/owner/repo`, and the
host has to be one that is known to hold repositories. Ordinary text is left
alone.

Inside a repository the clipboard is not read at all, because there is nothing
to clone.

### The picker starts outside a repository

It used to refuse, and exit with "not inside a git repository". Now it opens
in an empty state, so there is somewhere to clone from. Leaving without a
repository still exits with 1, because there was nothing to work on.

### Every operation shows what git said

After a git command finishes, what it said is on screen:

```
? repo  git pull

+-- git pull ------------------------------
| stdout:
| From github.com:owner/repo
|  * [new branch]  feature/x -> feature/x
| stderr:
| ...
+---------------------------------------

esc closes the output, arrows scroll (12 of 12)
```

The output scrolls with the arrows and the page keys, and escape closes it.

A switch and the pull that follows it are one panel, because they are one
thing that was asked for. The background fetch has no panel, because it
happens before you have done anything, and the file list has no panel either,
because that output is already on screen as a list.

The question about pushing a commit comes after the commit output has been
read, and the question about the pull request page comes after the push
output.

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
