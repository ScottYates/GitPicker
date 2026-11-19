## git-picker v1.6

After a push to GitHub, the picker offers to open the pull request page.

### The pull request page

A push to GitHub ends with git printing the address of a pull request. The
picker picks that up and asks whether to open it:

```
? repo  Open the pull request page?

> yes
  no

https://github.com/owner/repo/compare/develop...feature/x?quick_pull=1
y or n, enter to choose, esc to skip
```

Answer yes and the page opens in your default browser. Answer no, or press
escape, and the picker carries on.

It happens after any push, whether it came from the commit flow or from
Ctrl+P. A push that was not to GitHub has no page to offer, so nothing is
asked.

### The pull request goes back where the work came from

The page is opened with the base branch already chosen, so the pull request
goes back to the branch the work was branched from, rather than to the
default branch of the repository.

Git records that in the reflog of the branch, as `branch: Created from
main`, and a branch made with no starting point is recorded as `Created from
HEAD`, which the HEAD reflog then explains. A branch that is not on the
remote is left out, because GitHub can only make a pull request against a
branch that is in the repository, and GitHub picks its default in that case.

When the branch already has a pull request, that pull request is opened as
it is, because its base branch was already chosen when it was made.

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
| y / n | answer the push question, and the pull request question |
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
