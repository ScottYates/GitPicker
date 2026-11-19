#!/usr/bin/env python3
"""git-picker: interactive git branch picker that switches and pulls.

Version: v1.10

Finds the repository root, fetches and prunes, lists local and remote
branches in a fuzzy-selectable menu, switches to the chosen branch and
pulls. Remote-only branches are offered and checked out with --track.

The picker fills the whole screen, reflows when the window is resized,
pages with PageUp/PageDown, shows its position in the list, shows the
repository folder name at the top, and displays an ASCII panel with the
exact commands it will run. Escape and Ctrl+C both close it.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass
from typing import Any, Callable

from InquirerPy.prompts.fuzzy import FuzzyPrompt
from InquirerPy.utils import get_style
from pfzy.score import substr_scorer
from prompt_toolkit.filters.cli import IsDone
from prompt_toolkit.layout.containers import ConditionalContainer, Window
from prompt_toolkit.layout.controls import UIContent, UIControl
from prompt_toolkit.layout.dimension import Dimension
from prompt_toolkit.utils import get_cwidth

# The command panel is a title rule plus a two-line body. It is open on the
# right and bottom, and only counts while the prompt is still unanswered.
PREVIEW_ROWS = 3
MIN_PANEL_WIDTH = 24

REMOTE_TAG = "[remote]"
REMOTE_WIDTH = len(REMOTE_TAG)
# "11mo" is the widest age ever rendered, so four cells is the natural column.
AGE_WIDTH = 4

SORT_TIME = "time"
SORT_NAME = "name"

BOX_TOP_LEFT = "+"
BOX_HORIZONTAL = "-"
BOX_VERTICAL = "|"

# Colours for the command panel. Merged over InquirerPy's default theme, so
# nothing else about the prompt changes.
PREVIEW_STYLE = {
    "preview.border": "#5c6370",
    "preview.title": "#e5c07b",
    "preview.cmd": "#98c379",
    "preview.warn": "#e06c75",
    "preview.idle": "#5c6370",
}

# Branches that always sort to the top, in this exact order.
PINNED_ORDER = (
    "main",
    "master",
    "develop",
    "dev",
    "uat",
    "test",
    "prod",
    "production",
)

# Lookup is case-insensitive: repos in the wild spell these PROD, Uat, Dev.
_PINNED_RANK = {name: index for index, name in enumerate(PINNED_ORDER)}
_UNPINNED_RANK = len(PINNED_ORDER)

DEFAULT_REMOTE = "origin"

# Fields come back tab separated: full refname, unix commit date, upstream.
# The refname is the FULL one on purpose; see branch_name_from_ref.
REF_FORMAT = "%(refname)\t%(committerdate:unix)\t%(upstream:short)"
REF_FORMAT_ARG = f"--format={REF_FORMAT}"

LOCAL_REF_PREFIX = "refs/heads/"
REMOTE_REF_PREFIX = f"refs/remotes/{DEFAULT_REMOTE}/"
REMOTE_HEAD_REF = f"refs/remotes/{DEFAULT_REMOTE}/HEAD"

MINUTE = 60
HOUR = 60 * MINUTE
DAY = 24 * HOUR
MONTH = 30 * DAY
YEAR = 365 * DAY


class GitError(Exception):
    """A git command failed. Carries git's own exit code."""

    def __init__(self, code: int, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass
class Branch:
    """A selectable branch, either local or present only on the remote."""

    name: str
    remote: bool
    timestamp: int
    upstream: str = ""


def run_git(args: list[str], cwd: str | None = None) -> subprocess.CompletedProcess:
    """Run git, capturing output so it never leaks into the menu UI."""
    return subprocess.run(
        ["git", *args],
        cwd=cwd,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
    )


def check_git(proc: subprocess.CompletedProcess, action: str) -> str:
    """Return stdout, or raise GitError carrying git's exit code."""
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout or "").strip()
        raise GitError(proc.returncode, f"git {action} failed: {detail}")
    return proc.stdout


def long_path(path: str) -> str:
    """Expand a Windows 8.3 short path into its real long form.

    A process launched from a short path keeps that spelling in os.getcwd(),
    so the header would show 'BTS-MA~2' instead of the folder name.
    GetLongPathNameW is the documented way to undo it. Every failure returns
    the input unchanged: a wrong header is far better than a picker that
    refuses to start.
    """
    if os.name != "nt" or not path:
        return path
    try:
        import ctypes

        get_long_path_name = ctypes.windll.kernel32.GetLongPathNameW
        get_long_path_name.argtypes = [ctypes.c_wchar_p, ctypes.c_wchar_p, ctypes.c_uint]
        get_long_path_name.restype = ctypes.c_uint

        # A zero-length buffer asks the API for the size it needs, counting
        # the terminating null. Zero back means the path does not resolve.
        needed = get_long_path_name(path, None, 0)
        if not needed:
            return path
        buf = ctypes.create_unicode_buffer(needed)
        if not get_long_path_name(path, buf, needed):
            return path
        return buf.value or path
    except Exception:
        return path


def find_repo_root(start: str) -> str | None:
    """Walk upward from start until a .git entry is found.

    Accepts both a .git directory and the .git file that worktrees and
    submodules use, so linked checkouts resolve correctly too. The start
    path is expanded to its long form first, so the returned root (and the
    header derived from it) never carries an 8.3 alias.
    """
    current = long_path(os.path.abspath(start))
    while True:
        if os.path.exists(os.path.join(current, ".git")):
            return current
        parent = os.path.dirname(current)
        if parent == current:
            return None
        current = parent


def fetch(cwd: str) -> None:
    check_git(run_git(["fetch", "--all", "--prune"], cwd), "fetch")


def branch_name_from_ref(ref: str) -> str:
    """Recover the true branch name from a full refname.

    Deliberately avoids %(refname:short). When a branch and a tag share a
    name, git disambiguates the short form to 'heads/feature/x', which is not
    a branch name you can switch to. Stripping the known prefix ourselves is
    exact regardless of tags or other refs sharing the name.
    """
    if ref.startswith(LOCAL_REF_PREFIX):
        return ref[len(LOCAL_REF_PREFIX):]
    if ref.startswith(REMOTE_REF_PREFIX):
        return ref[len(REMOTE_REF_PREFIX):]
    return ref


def parse_refs(stdout: str) -> list[tuple[str, int, str]]:
    """Turn for-each-ref output into (full refname, unix timestamp, upstream)."""
    refs = []
    for line in stdout.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        ref = parts[0].strip()
        if not ref:
            continue
        try:
            timestamp = int(parts[1].strip() or 0) if len(parts) > 1 else 0
        except ValueError:
            timestamp = 0
        upstream = parts[2].strip() if len(parts) > 2 else ""
        refs.append((ref, timestamp, upstream))
    return refs


def current_branch(cwd: str) -> str:
    """Name of the checked out branch, or '' on a detached/unborn HEAD.

    Reads the full ref via symbolic-ref rather than `rev-parse --abbrev-ref`:
    the abbreviated form disambiguates against tags, so a branch named
    feature/x alongside a tag of the same name would come back as
    'heads/feature/x' and the current-branch marker would never match.
    """
    proc = run_git(["symbolic-ref", "--quiet", "HEAD"], cwd)
    ref = proc.stdout.strip()
    if proc.returncode != 0 or not ref:
        return ""
    return branch_name_from_ref(ref)


def list_branches(cwd: str) -> list[Branch]:
    """Collect local branches plus remote-only branches from origin."""
    local_out = check_git(run_git(["for-each-ref", REF_FORMAT_ARG, LOCAL_REF_PREFIX], cwd), "for-each-ref refs/heads")
    local = {branch_name_from_ref(ref): (ts, up) for ref, ts, up in parse_refs(local_out)}

    remote_out = check_git(
        run_git(["for-each-ref", REF_FORMAT_ARG, REMOTE_REF_PREFIX.rstrip("/")], cwd),
        f"for-each-ref refs/remotes/{DEFAULT_REMOTE}",
    )
    remote: dict[str, int] = {}
    for ref, ts, _ in parse_refs(remote_out):
        if ref == REMOTE_HEAD_REF or not ref.startswith(REMOTE_REF_PREFIX):
            continue
        name = branch_name_from_ref(ref)
        if name:
            remote.setdefault(name, ts)

    branches = [Branch(name, False, ts, up) for name, (ts, up) in local.items()]
    branches += [Branch(name, True, ts) for name, ts in remote.items() if name not in local]
    return branches


def sort_branches(branches: list[Branch]) -> list[Branch]:
    """Pinned branches first in PINNED_ORDER, then newest commit first.

    Pinning matches case-insensitively, so PROD, Prod and prod all take the
    same slot instead of only the lowercase spelling being recognised.
    """
    return sorted(
        branches,
        key=lambda b: (_PINNED_RANK.get(b.name.lower(), _UNPINNED_RANK), -b.timestamp, b.name),
    )


def format_age(timestamp: int, now: float | None = None) -> str:
    """Compact relative age: 15s, 12m, 3h, 5d, 2mo, 1y."""
    if not timestamp:
        return "?"
    elapsed = int(max(0.0, (time.time() if now is None else now) - timestamp))
    if elapsed < MINUTE:
        return f"{elapsed}s"
    if elapsed < HOUR:
        return f"{elapsed // MINUTE}m"
    if elapsed < DAY:
        return f"{elapsed // HOUR}h"
    if elapsed < MONTH:
        return f"{elapsed // DAY}d"
    if elapsed < YEAR:
        return f"{elapsed // MONTH}mo"
    return f"{elapsed // YEAR}y"


def switch_args(branch: Branch) -> list[str]:
    """The git subcommand that checks out this branch (no leading `git`)."""
    if branch.remote:
        return ["switch", "--track", f"{DEFAULT_REMOTE}/{branch.name}"]
    return ["switch", branch.name]


def switch_command(branch: Branch) -> str:
    """The full command shown in the preview panel.

    Derived from switch_args so the preview cannot drift from what runs.
    Git ref names cannot contain spaces, so a plain join is exact.
    """
    return "git " + " ".join(switch_args(branch))


def fit_width(text: str, width: int) -> str:
    """Truncate to width display cells, marking that it was cut."""
    if get_cwidth(text) <= width:
        return text
    limit = max(1, width - 3)
    out: list[str] = []
    used = 0
    for char in text:
        size = get_cwidth(char)
        if used + size > limit:
            break
        out.append(char)
        used += size
    return "".join(out) + "..."


def pad_to_width(text: str, width: int) -> str:
    """Truncate if needed, then pad with spaces to exactly width cells."""
    text = fit_width(text, width)
    used = get_cwidth(text)
    if used < width:
        text += " " * (width - used)
    return text


def pad_cells(text: str, width: int) -> str:
    """Pad on the right to exactly width cells. Never truncates."""
    return text + " " * max(0, width - get_cwidth(text))


def rjust_cells(text: str, width: int) -> str:
    """Pad on the left to exactly width cells. Never truncates."""
    return " " * max(0, width - get_cwidth(text)) + text


def label_name_width(branches: list[Branch]) -> int:
    """Widest branch name, so the name column lines up for every row."""
    return max((get_cwidth(b.name) for b in branches), default=0)


def display_label(branch: Branch, current: str, now: float | None = None, name_width: int | None = None) -> str:
    """Render one menu row as aligned table columns.

    Name, [remote] and age are each padded to a fixed width so the columns
    line up down the list. The age is right-aligned, which is what makes the
    units read as a column rather than trailing noise.
    """
    marker = "* " if branch.name == current else "  "
    width = label_name_width([branch]) if name_width is None else name_width
    tag = REMOTE_TAG if branch.remote else ""
    age = format_age(branch.timestamp, now)
    return f"{marker}{pad_cells(branch.name, width)}  {pad_cells(tag, REMOTE_WIDTH)}  {rjust_cells(age, AGE_WIDTH)}"


class CommandPreviewControl(UIControl):
    """Draws the command panel at the bottom of the prompt.

    The panel is open on the right and bottom: a title rule and the command
    lines, each prefixed with a bar. The rule is drawn by hand rather than
    with prompt_toolkit's Frame, whose own style handling raises
    `Wrong color format` while rendering, and drawing it here keeps every
    row pinned to an exact width.

    Deliberately not a FormattedTextControl either, because that caches its
    text per render and this has to track the highlighted row every redraw.
    """

    def __init__(self, provider: Callable[[int], list[tuple[str, str]]], title: str) -> None:
        self._provider = provider
        self._title = title

    def create_content(self, width: int, height: int) -> UIContent:
        width = max(MIN_PANEL_WIDTH, width)
        title = self._title
        head = BOX_TOP_LEFT + BOX_HORIZONTAL * 2 + " "
        tail = max(0, width - get_cwidth(head) - get_cwidth(title) - 1)

        # Open on the right and bottom: the panel is a preview, not a modal,
        # and an edge sitting at the terminal margin wraps on some consoles.
        rows: list[list[tuple[str, str]]] = [
            [
                ("class:preview.border", head),
                ("class:preview.title", title),
                ("class:preview.border", " " + BOX_HORIZONTAL * tail),
            ]
        ]
        for style, text in self._provider(width - 2):
            rows.append([
                ("class:preview.border", BOX_VERTICAL + " "),
                (style, pad_to_width(text, width - 2)),
            ])

        return UIContent(
            get_line=lambda index: rows[index],
            line_count=len(rows),
            show_cursor=False,
            cursor_position=None,
        )


class BranchPicker(FuzzyPrompt):
    """FuzzyPrompt that fills the screen, reflows on resize, and pages.

    Three additions over the stock prompt:

    * The list takes every free terminal row instead of the default 70%.
    * That height is recomputed from the live terminal size on each layout
      pass, so maximizing or resizing the window reflows the list instead of
      leaving it clipped at whatever the size was when it opened.
    * PageUp/PageDown move a screenful at a time, and a live readout shows
      where the pointer sits in the list.
    """

    def __init__(
        self,
        *args: Any,
        branch_lookup: dict[str, Branch] | None = None,
        sort_mode: str = SORT_TIME,
        **kwargs: Any,
    ) -> None:
        kwargs.setdefault("style", get_style(PREVIEW_STYLE, style_override=False))
        super().__init__(*args, **kwargs)
        self._branch_lookup = branch_lookup or {}
        self._applied_rows = 0
        self.sort_mode = sort_mode if sort_mode in (SORT_TIME, SORT_NAME) else SORT_TIME
        # Replacing the fixed dimension with a callable is what makes the
        # reflow work: prompt_toolkit re-runs this on every layout pass, which
        # includes the pass it does after a terminal resize.
        self.choice_window.height = self._height_dimension
        # The list control sizes itself from its own _height/_max_lines, which
        # are otherwise fixed once at startup. Its create_content is the only
        # hook that runs before line_count is computed, so re-sync there and
        # the visible window actually tracks the terminal size.
        control = self.content_control
        original_create_content = control.create_content

        def create_content(width: int, height: int | None) -> Any:
            self._sync_list_height(height)
            return original_create_content(width, height)

        control.create_content = create_content
        self._install_command_panel()
        self._sync_list_height()
        self._register_paging_keys()

    def _install_command_panel(self) -> None:
        """Slot the command panel in between the list and the instructions.

        The stock layout is [message, list, spacer, instructions], so the panel
        goes in at index 2. Its height is added to the prompt's height offset
        so the list still knows how many rows it really has.
        """
        self._panel_control = CommandPreviewControl(self._command_preview_lines, "will run")
        panel = Window(
            content=self._panel_control,
            height=Dimension.exact(PREVIEW_ROWS),
            dont_extend_height=True,
            wrap_lines=False,
        )
        self._height_offset += PREVIEW_ROWS
        split = self._layout.container.content
        split.children.insert(2, ConditionalContainer(content=panel, filter=~IsDone()))

    def selected_branch(self) -> Branch | None:
        """The Branch under the pointer, or None if nothing is selectable."""
        if not self._branch_lookup:
            return None
        control = self.content_control
        if not control.choice_count:
            return None
        try:
            label = control.selection["name"]
        except IndexError:
            return None
        return self._branch_lookup.get(label)

    def _command_preview_lines(self, width: int) -> list[tuple[str, str]]:
        """The two commands the picker will run for the highlighted branch."""
        branch = self.selected_branch()
        if branch is None:
            return [("class:preview.idle", "nothing selected")]
        lines = [("class:preview.cmd", fit_width(switch_command(branch), width))]
        if branch.remote or branch.upstream:
            lines.append(("class:preview.cmd", "git pull"))
        else:
            lines.append(("class:preview.warn", fit_width("git pull  (no upstream, will fail)", width)))
        return lines

    def _terminal_rows(self) -> int:
        """Live terminal height, falling back to the OS-reported size."""
        output = getattr(getattr(self, "_application", None), "output", None)
        if output is not None:
            try:
                rows = output.get_size().rows
            except Exception:  # noqa: BLE001 - size probing must never break the prompt
                rows = 0
            if rows:
                return rows
        return shutil.get_terminal_size().lines

    def _list_rows(self) -> int:
        """Rows the list may occupy once the chrome around it is subtracted."""
        return max(1, self._terminal_rows() - self.height_offset)

    def _height_dimension(self) -> Dimension:
        """Desired height for the choice window, re-evaluated per layout pass.

        Deliberately asks for the whole terminal rather than
        `terminal - height_offset`: the split then gives the window whatever
        the fixed-size message and input rows leave behind, which fills the
        last screen row instead of leaving it blank.
        """
        available = max(1, self._terminal_rows())
        return Dimension(min=1, preferred=available, max=available)

    def _sync_list_height(self, allocated: int | None = None) -> None:
        """Point the control's scroll window at the rows it really has.

        `allocated` is the height the layout actually gave the choice window,
        which is the only number guaranteed to match the screen. During layout
        probing it is None and the estimate from the terminal size is used.

        The control caches its rendered rows per render, not per height, so a
        resize would otherwise keep replaying the old row count and the list
        would stay short instead of reflowing. Dropping the caches whenever
        the height changes is what makes maximizing actually take effect.
        """
        control = self.content_control
        available = allocated if allocated and allocated > 0 else self._list_rows()
        if available == self._applied_rows:
            return
        self._applied_rows = available
        control._max_lines = available
        control._height = min(available, max(control.choice_count, 1))
        control._fragment_cache.clear()
        control._content_cache.clear()

    def _page_rows(self) -> int:
        """Rows to travel per page, keeping one line of context."""
        available = self._applied_rows if self._applied_rows > 0 else self._list_rows()
        return max(1, available - 1)

    def _register_paging_keys(self) -> None:
        """Attach PageUp/PageDown to a screenful-at-a-time movement."""

        @self.register_kb("pageup")
        def _page_up(_event: Any) -> None:
            self._move_page(-1)

        @self.register_kb("pagedown")
        def _page_down(_event: Any) -> None:
            self._move_page(1)

        @self.register_kb("c-s")
        def _toggle_sort(_event: Any) -> None:
            # Ctrl-S rather than a plain letter: a printable key would be
            # swallowed instead of reaching the search buffer.
            self.toggle_sort()

        @self.register_kb("escape")
        def _escape(event: Any) -> None:
            # Reuse the Ctrl+C path so Escape cancels exactly the same way.
            self._handle_interrupt(event)

    def toggle_sort(self) -> None:
        """Flip between newest-first and alphabetical, keeping pins on top."""
        self.sort_mode = SORT_NAME if self.sort_mode == SORT_TIME else SORT_TIME
        self._apply_sort()

    def _sort_key(self, choice: dict) -> tuple:
        """Ordering for one choice. Pinned branches always lead, in both modes."""
        branch = self._branch_lookup.get(choice.get("name", ""))
        name = choice.get("name", "")
        if branch is None:
            return (_UNPINNED_RANK, name.lower(), name)
        rank = _PINNED_RANK.get(branch.name.lower(), _UNPINNED_RANK)
        if self.sort_mode == SORT_NAME:
            return (rank, branch.name.lower(), branch.name)
        return (rank, -branch.timestamp, branch.name)

    def _apply_sort(self) -> None:
        """Re-order the displayed choices in place and keep the selection."""
        control = self.content_control
        selected = control.selection["name"] if control.choice_count else None

        visible = {id(choice) for choice in control._filtered_choices}
        ordered = sorted(control._choices, key=self._sort_key)
        for position, choice in enumerate(ordered):
            choice["index"] = position
        control._choices = ordered
        control._filtered_choices = [choice for choice in ordered if id(choice) in visible]

        for position, choice in enumerate(control._filtered_choices):
            if choice.get("name") == selected:
                control._selected_choice_index = position
                break

        control._first_line = 0
        control._last_line = min(control._height, control.choice_count)
        control._fragment_cache.clear()
        control._content_cache.clear()
        self._redraw()

    def _move_page(self, direction: int) -> None:
        """Shift the pointer by a page, stopping at either end.

        Paging clamps rather than wrapping even when `cycle` is on: single-step
        arrows wrapping is handy, but wrapping a screenful at a time throws you
        back to the top of the list for no reason.
        """
        control = self.content_control
        count = control.choice_count
        if count <= 0:
            return
        target = control.selected_choice_index + direction * self._page_rows()
        control.selected_choice_index = max(0, min(target, count - 1))
        self._redraw()

    def _generate_after_input(self) -> list[tuple[str, str]]:
        """Live pointer position, sort mode, plus a match count while filtering."""
        if not self._info:
            return []
        control = self.content_control
        total = len(control.choices)
        count = control.choice_count
        position = control.selected_choice_index + 1 if count else 0
        fragments: list[tuple[str, str]] = [
            ("", "  "),
            ("class:fuzzy_info", f"[{position}/{count}]"),
            ("class:fuzzy_info", f"  sort: {self.sort_mode}"),
        ]
        if count != total:
            fragments.append(("class:fuzzy_info", f" ({count} of {total} match)"))
        if control._scorer == substr_scorer:
            fragments.append(("class:fuzzy_info", self._exact_symbol))
        return fragments


def repo_label(root: str) -> str:
    """Folder name to show at the top of the picker.

    Expands an 8.3 short path first, since a repo reached as 'BTS-MA~2'
    would otherwise be labelled with the alias rather than the folder name.
    Falls back to the full path when the repo sits at a drive root, where
    the basename is empty and the header would otherwise be blank.
    """
    expanded = long_path(root.rstrip("\\/"))
    # Fall back to the ORIGINAL root, not the stripped/expanded one. At a
    # drive root the stripped form is 'C:', whose basename is empty, so
    # returning it would print a drive-relative path instead of 'C:\'.
    return os.path.basename(expanded) or root


def choose_branch(
    branches: list[Branch],
    current: str,
    sort_mode: str = SORT_TIME,
    repo_name: str = "",
) -> Branch | None:
    """Show the fuzzy picker. Returns None when the user cancels."""
    from InquirerPy.base.control import Choice

    # One shared name width so the columns line up across every row. Branch
    # names are unique per ref, so the labels are unique and can key the map.
    width = label_name_width(branches)
    labels = {display_label(b, current, None, width): b for b in branches}

    # Three deliberate choices here:
    #  - No `default`: FuzzyPrompt seeds the search buffer with it, which would
    #    pre-filter the list down to the current branch only.
    #  - `value` is the label string, not the Branch: the prompt builds its
    #    return value from the value object's attributes, so passing the
    #    dataclass comes back as a dict instead of a Branch.
    #  - Choices arrive pre-sorted; Ctrl-S re-orders them live.
    # Constructing a prompt does not run it; execute() does.
    try:
        selected = BranchPicker(
            message=f"{repo_name}  Switch to branch:" if repo_name else "Switch to branch:",
            choices=[Choice(name=label, value=label) for label in labels],
            branch_lookup=labels,
            sort_mode=sort_mode,
            long_instruction="type to filter, arrows/pgup/pgdn, ctrl+s sort, enter to switch, esc cancels",
        ).execute()
    except KeyboardInterrupt:
        return None

    if isinstance(selected, dict):
        selected = selected.get("name", "")
    return labels.get(selected) or labels.get(str(selected).strip())


def switch_branch(branch: Branch, cwd: str) -> None:
    """Check out a local branch, or track a remote-only one."""
    args = switch_args(branch)
    check_git(run_git(args, cwd), " ".join(args))


def pull_command(branch: Branch) -> str:
    """The pull the picker runs after switching. A remote branch gets tracking."""
    return "git pull" if branch.remote or branch.upstream else "git pull  # fails, no upstream"


def pull(cwd: str) -> None:
    check_git(run_git(["pull"], cwd), "pull")


def run() -> int:
    root = find_repo_root(os.getcwd())
    if root is None:
        print("Error: not inside a git repository.", file=sys.stderr)
        return 1
    os.chdir(root)

    fetch(root)
    branches = sort_branches(list_branches(root))
    if not branches:
        print("Error: no branches found.", file=sys.stderr)
        return 1

    chosen = choose_branch(branches, current_branch(root), repo_name=repo_label(root))
    if chosen is None:
        print("Cancelled.")
        return 130

    switch_branch(chosen, root)
    print(f"Switched to {chosen.name}.")
    pull(root)
    return 0


def main() -> int:
    try:
        return run()
    except GitError as error:
        print(f"Error: {error}", file=sys.stderr)
        return error.code
    except KeyboardInterrupt:
        print("\nCancelled.", file=sys.stderr)
        return 130
    except Exception as error:  # noqa: BLE001 - top level guard, reported as exit 1
        print(f"Error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())