## git-picker v1.1

A standalone Windows build of the branch picker. One file, no runtime to
install, and the branch list is on screen in about 41 ms.

### What it does

Finds the repository root, lists local and remote branches in a
fuzzy-selectable menu, switches to the one you pick and pulls. Remote-only
branches are offered and checked out with `--track`.

### Keys

| Key | What it does |
| --- | --- |
| type | filter the list |
| up / down | move the pointer |
| pgup / pgdn | move a screenful |
| ctrl+s | sort by time or by name, pinned branches first |
| ctrl+n | switch to the highlighted branch and make a new one from it |
| enter | switch to the highlighted branch and pull |
| esc | leave |

### Notes

- The fetch runs in the background, so the list is on screen without waiting
  for the network, and it refreshes when the fetch lands.
- Switching does not close the picker. The branch is switched, the list is
  rebuilt and the picker carries on, so branches can be worked through one
  after another.
- The list fills the screen and reflows when the window is resized.

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
