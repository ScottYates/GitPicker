package main

// The branch model: what a row shows, how rows are ordered, and the exact
// git commands the picker will run. These mirror git_picker.py so the Go
// picker behaves the same as the Python one.

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	remoteTag   = "[remote]"
	remoteWidth = len(remoteTag)

	// "11mo" is the widest age ever rendered, so four cells is the natural
	// column.
	ageWidth = 4

	sortTime = "time"
	sortName = "name"

	defaultRemote = "origin"

	localRefPrefix  = "refs/heads/"
	remoteRefPrefix = "refs/remotes/" + defaultRemote + "/"
	remoteHeadRef   = "refs/remotes/" + defaultRemote + "/HEAD"

	minute = 60
	hour   = 60 * minute
	day    = 24 * hour
	month  = 30 * day
	year   = 365 * day
)

// Branches that always sort to the top, in this exact order.
var pinnedOrder = []string{
	"main",
	"master",
	"develop",
	"dev",
	"uat",
	"test",
	"prod",
	"production",
}

// pinnedRank is the sort slot of a branch name.
//
// Lookup is case-insensitive: repos in the wild spell these PROD, Uat, Dev,
// and all of those take the same slot rather than only the lowercase
// spelling being recognised. The rank doubles as the "not pinned" value.
func pinnedRank(name string) int {
	lower := strings.ToLower(name)
	for i, pinned := range pinnedOrder {
		if lower == pinned {
			return i
		}
	}
	return len(pinnedOrder)
}

// Branch is a selectable branch, either local or present only on the remote.
type Branch struct {
	Name      string
	Remote    bool
	Timestamp int64
	Upstream  string
}

// sortBranches orders the branches the way the picker shows them.
//
// Pinned branches lead in pinnedOrder, then the rest follow newest commit
// first, with the name as the final tie break.
func sortBranches(branches []Branch, mode string) {
	sort.SliceStable(branches, func(i, j int) bool {
		ri, rj := pinnedRank(branches[i].Name), pinnedRank(branches[j].Name)
		if ri != rj {
			return ri < rj
		}
		if mode == sortName {
			li, lj := strings.ToLower(branches[i].Name), strings.ToLower(branches[j].Name)
			if li != lj {
				return li < lj
			}
			return branches[i].Name < branches[j].Name
		}
		if branches[i].Timestamp != branches[j].Timestamp {
			return branches[i].Timestamp > branches[j].Timestamp
		}
		return branches[i].Name < branches[j].Name
	})
}

// hoistCurrent puts the branch that is checked out at the top of the list.
//
// It is the branch the reader is most likely to want to see, so it leads
// whatever the sort or the query says. It is only there while the query
// matches it, because a filter that does not match it is the reader saying
// that they do not want to see it.
func hoistCurrent(matches []match, branches []Branch, current string) []match {
	if current == "" {
		return matches
	}
	for i, entry := range matches {
		if branches[entry.index].Name != current {
			continue
		}
		if i == 0 {
			return matches
		}
		// The rows above it move down one to make room.
		copy(matches[1:i+1], matches[0:i])
		matches[0] = entry
		return matches
	}
	return matches
}

// formatAge is a compact relative age: 15s, 12m, 3h, 5d, 2mo, 1y.
func formatAge(timestamp int64, now int64) string {
	if timestamp == 0 {
		return "?"
	}
	elapsed := now - timestamp
	if elapsed < 0 {
		elapsed = 0
	}
	switch {
	case elapsed < minute:
		return fmt.Sprintf("%ds", elapsed)
	case elapsed < hour:
		return fmt.Sprintf("%dm", elapsed/minute)
	case elapsed < day:
		return fmt.Sprintf("%dh", elapsed/hour)
	case elapsed < month:
		return fmt.Sprintf("%dd", elapsed/day)
	case elapsed < year:
		return fmt.Sprintf("%dmo", elapsed/month)
	default:
		return fmt.Sprintf("%dy", elapsed/year)
	}
}

// switchArgs is the git subcommand that checks out this branch, without the
// leading `git`.
func switchArgs(branch Branch) []string {
	if branch.Remote {
		return []string{"switch", "--track", defaultRemote + "/" + branch.Name}
	}
	return []string{"switch", branch.Name}
}

// switchCommand is the full command shown in the preview panel.
//
// Derived from switchArgs so the preview cannot drift from what runs. Git
// ref names cannot contain spaces, so a plain join is exact.
func switchCommand(branch Branch) string {
	return "git " + strings.Join(switchArgs(branch), " ")
}

// pullCommand is the pull the picker runs after switching.
func pullCommand(branch Branch) string {
	if branch.Remote || branch.Upstream != "" {
		return "git pull"
	}
	return "git pull  # fails, no upstream"
}

// displayWidth is the number of terminal cells a string occupies.
//
// This is the Go side of prompt_toolkit's get_cwidth: combining marks take
// no space and East Asian wide characters take two.
func displayWidth(s string) int {
	total := 0
	for _, r := range s {
		total += runeWidth(r)
	}
	return total
}

// runeWidth is the cell width of a single rune.
func runeWidth(r rune) int {
	switch {
	case r < 0x1100:
		return 1
	case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), unicode.Is(unicode.Cf, r):
		return 0
	case isWide(r):
		return 2
	default:
		return 1
	}
}

// isWide reports whether a rune is an East Asian wide or fullwidth character.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E,   // CJK radicals, Kangxi, punctuation
		r >= 0x3041 && r <= 0x33FF,   // Hiragana, Katakana, CJK compatibility
		r >= 0x3400 && r <= 0x4DBF,   // CJK extension A
		r >= 0x4E00 && r <= 0x9FFF,   // CJK unified ideographs
		r >= 0xA000 && r <= 0xA4CF,   // Yi
		r >= 0xAC00 && r <= 0xD7A3,   // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF,   // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE6F,   // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60,   // Fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6,   // Fullwidth signs
		r >= 0x1F300 && r <= 0x1F64F, // Emoji
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

// fitWidth truncates to width display cells, marking that it was cut.
func fitWidth(text string, width int) string {
	if displayWidth(text) <= width {
		return text
	}
	limit := width - 3
	if limit < 1 {
		limit = 1
	}
	var out strings.Builder
	used := 0
	for _, r := range text {
		size := runeWidth(r)
		if used+size > limit {
			break
		}
		out.WriteRune(r)
		used += size
	}
	out.WriteString("...")
	return out.String()
}

// padToWidth truncates if needed, then pads with spaces to exactly width
// cells.
func padToWidth(text string, width int) string {
	text = fitWidth(text, width)
	if used := displayWidth(text); used < width {
		text += strings.Repeat(" ", width-used)
	}
	return text
}

// padCells pads on the right to exactly width cells. Never truncates.
func padCells(text string, width int) string {
	if used := displayWidth(text); used < width {
		text += strings.Repeat(" ", width-used)
	}
	return text
}

// rjustCells pads on the left to exactly width cells. Never truncates.
func rjustCells(text string, width int) string {
	if used := displayWidth(text); used < width {
		text = strings.Repeat(" ", width-used) + text
	}
	return text
}

// labelNameWidth is the widest branch name, so the name column lines up for
// every row.
func labelNameWidth(branches []Branch) int {
	width := 0
	for _, b := range branches {
		if w := displayWidth(b.Name); w > width {
			width = w
		}
	}
	return width
}

// displayLabel renders one menu row as aligned table columns.
//
// Name, [remote] and age are each padded to a fixed width so the columns line
// up down the list. The age is right aligned, which is what makes the units
// read as a column rather than trailing noise.
func displayLabel(branch Branch, current string, now int64, nameWidth int) string {
	marker := "  "
	if branch.Name == current {
		marker = "* "
	}
	tag := ""
	if branch.Remote {
		tag = remoteTag
	}
	age := formatAge(branch.Timestamp, now)
	return marker +
		padCells(branch.Name, nameWidth) + "  " +
		padCells(tag, remoteWidth) + "  " +
		rjustCells(age, ageWidth)
}

// repoLabel is the folder name shown at the top of the picker.
//
// Expands an 8.3 short path first, since a repo reached as 'BTS-MA~2' would
// otherwise be labelled with the alias rather than the folder name. Falls
// back to the full path when the repo sits at a drive root, where the
// basename is empty and the header would otherwise be blank.
func repoLabel(root string) string {
	expanded := longPath(strings.TrimRight(root, `\/`))
	// Fall back to the ORIGINAL root, not the stripped or expanded one. At a
	// drive root the stripped form is 'C:', whose basename is empty, so
	// returning it would print a drive-relative path instead of 'C:\'.
	name := ntBase(expanded)
	if name == "" {
		return root
	}
	return name
}

// ntBase is the Go equivalent of Python's ntpath.basename, which the Python
// picker's repo_label depends on.
//
// The difference that matters is the empty result for a drive root and for a
// bare UNC share: those are exactly the cases where the Python picker falls
// back to printing the full path, and the Go picker has to agree or the
// header changes.
func ntBase(path string) string {
	rest := path
	if len(rest) >= 2 && rest[1] == ':' {
		rest = rest[2:]
	}
	if strings.HasPrefix(rest, `\\`) {
		// A UNC share is a drive: the server and the share are consumed by
		// it, so "\\server\share" has no basename of its own.
		body := rest[2:]
		sep := strings.IndexAny(body, `\/`)
		if sep < 0 {
			return ""
		}
		afterServer := body[sep+1:]
		sep2 := strings.IndexAny(afterServer, `\/`)
		if sep2 < 0 {
			return ""
		}
		rest = afterServer[sep2+1:]
	}
	if strings.Trim(rest, `\/`) == "" {
		// A drive root such as "C:\" has no basename.
		return ""
	}
	if i := strings.LastIndexAny(rest, `\/`); i >= 0 {
		return rest[i+1:]
	}
	return rest
}

// nowSeconds is the clock used for the age column, injectable so tests can
// pin it.
var nowSeconds = func() int64 { return time.Now().Unix() }
