## git-picker v1.2

Fixes the control keys, and takes the slow pull off the critical path.

### The control keys

`ctrl+s` and `ctrl+n` did nothing, and pressing a control key seemed to
change the display instead.

The cause was in how the terminal delivers the key. When a console has
virtual terminal input enabled, `ctrl+n` arrives as the raw control byte
`0x0e` with no modifier flag attached. Bubbletea only turns that into the
`ctrl+n` key when the console reports that ctrl is held, so the byte came
through as an ordinary character and was typed into the search box. The
search box then filtered the list down to nothing, which is why the display
changed, why the new branch prompt refused to open (there was nothing left
to branch from) and why `ctrl+s` did not sort anything.

The picker now works out which combination a control character stands for
and acts on it, and a control character can no longer end up in the search
box or in a branch name.

### Switching is instant now

`git pull` is the slow part of a switch, because it is the one that talks to
the network. It used to hold up the whole picker, and every key was ignored
while it ran, so a `ctrl+n` pressed during the wait was simply thrown away.

- The branch is switched first, and the list updates straight away.
- The pull runs in the background, and the status line says when it lands.
- The keys keep working while it runs. Escape always leaves, even then.
- A switch that arrives during a pull waits its turn rather than racing it.
- The status line shows how long the switch and the pull each took.

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
