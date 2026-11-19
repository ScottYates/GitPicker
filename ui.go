package main

// The picker UI.
//
// The layout is the one the Python picker draws:
//
//	row 1        ? <repo>  Switch to branch:
//	row 2        pointer <query>  [pos/count]  sort: mode
//	rows 3..n    the branch list, one row per branch
//	row n-2      +-- will run ---------------
//	row n-1      | git switch <branch>
//	row n        | git pull
//	row n+1      the instructions
//
// The list takes every row the chrome does not need, so it fills the screen
// and reflows when the window is resized.
//
// Switching does not close the picker. The branch is switched, the list is
// rebuilt from what git now reports and the picker carries on, so branches
// can be worked through one after another. Escape leaves.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// pointer is the marker in front of the highlighted row.
	pointer = "\u276f"

	// previewRows is the height of the command panel: a title rule and two
	// command lines.
	previewRows = 3

	// minPanelWidth is the narrowest the panel is allowed to get.
	minPanelWidth = 24

	// chromeRows are the rows the picker spends on everything except the
	// list: the message, the search line, the panel and the instructions.
	chromeRows = 3 + previewRows

	instruction = "type to filter, arrows/pgup/pgdn, ctrl+s sort, ctrl+n new branch, enter to switch, esc exits"
)

var (
	stylePointer = lipgloss.NewStyle().Foreground(lipgloss.Color("#61afef"))
	styleMatch   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b")).Bold(true)
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5c6370"))
	styleBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5c6370"))
	styleTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b"))
	styleCmd     = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379"))
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75"))
	styleCurrent = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Bold(true)
	styleGood    = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Bold(true)
)

// phase is which part of the picker has the keyboard.
type phase int

const (
	phaseList phase = iota
	phaseName
)

// fetchResult is what the background fetch reports back.
type fetchResult struct {
	branches []Branch
	err      error
}

// fetchDoneMsg carries the result of the background fetch.
type fetchDoneMsg struct{ fetchResult }

// switchDoneMsg reports the result of a switch, and the branch the picker
// ended up on.
//
// The pull is reported separately from the switch, because a branch with no
// upstream fails its pull while the switch itself worked perfectly well.
type switchDoneMsg struct {
	branch  string
	newName string
	pullErr error
	err     error
}

// model is the picker state.
type model struct {
	root     string
	repoName string
	current  string

	branches  []Branch
	labels    []string
	nameWidth int

	matches []match
	query   string
	cursor  int
	offset  int

	sortMode string

	width  int
	height int
	now    int64

	phase     phase
	nameBuf   string
	nameErr   string
	fetchMsg  string
	status    string
	statusErr bool
	busy      bool

	fetchCh     chan fetchResult
	interrupted bool
}

// newModel builds the picker state from the branches already on disk.
func newModel(root, repoName, current string, branches []Branch, fetchCh chan fetchResult) model {
	m := model{
		root:     root,
		repoName: repoName,
		current:  current,
		sortMode: sortTime,
		fetchCh:  fetchCh,
		now:      nowSeconds(),
		width:    80,
		height:   24,
	}
	m.setBranches(branches)
	return m
}

// setBranches stores the branches, re-sorts them and rebuilds the labels.
func (m *model) setBranches(branches []Branch) {
	m.branches = branches
	sortBranches(m.branches, m.sortMode)
	m.nameWidth = labelNameWidth(m.branches)
	// The age column is relative to now, so the clock is read again
	// whenever the list is replaced.
	m.now = nowSeconds()
	m.rebuildLabels()
	m.refilter()
}

// rebuildLabels recomputes the display label of every branch.
//
// The label is what the fuzzy search matches against, exactly as in the
// Python picker, so a query can hit the age column or the [remote] tag.
func (m *model) rebuildLabels() {
	m.labels = make([]string, len(m.branches))
	for i := range m.branches {
		m.labels[i] = displayLabel(m.branches[i], m.current, m.now, m.nameWidth)
	}
}

// refilter re-applies the query to the branch list.
func (m *model) refilter() {
	m.matches = filterBranches(m.query, m.branches, m.labels)
	m.clampCursor()
}

// clampCursor keeps the pointer inside the list.
func (m *model) clampCursor() {
	if len(m.matches) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.matches) {
		m.cursor = len(m.matches) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// branchAt returns the branch behind a filtered match.
func (m model) branchAt(entry match) *Branch {
	return &m.branches[entry.index]
}

// listRows is how many rows the list may occupy.
func (m *model) listRows() int {
	rows := m.height - chromeRows
	if rows < 1 {
		rows = 1
	}
	return rows
}

// pageRows is how far PageUp and PageDown travel, keeping one line of
// context so the reader does not lose their place.
func (m *model) pageRows() int {
	rows := m.listRows() - 1
	if rows < 1 {
		rows = 1
	}
	return rows
}

// scrollIntoView moves the window so the pointer is visible.
func (m *model) scrollIntoView() {
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// Init starts the background fetch and waits for it.
func (m model) Init() tea.Cmd {
	ch := m.fetchCh
	return func() tea.Msg {
		result := <-ch
		return fetchDoneMsg{result}
	}
}

// Update handles the keyboard and the terminal size.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollIntoView()
		return m, nil

	case fetchDoneMsg:
		if msg.err != nil {
			m.fetchMsg = "fetch failed: " + firstLine(msg.err.Error())
		} else if len(msg.branches) > 0 {
			// Keep the pointer on the same branch across the refresh.
			selected := m.selectedName()
			m.setBranches(msg.branches)
			m.fetchMsg = ""
			m.selectByName(selected)
		}
		return m, nil

	case switchDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "error: " + firstLine(msg.err.Error())
			m.statusErr = true
			return m, nil
		}
		// The picker stays open, so the list and the current branch marker
		// have to catch up with what git just did.
		selected := m.selectedName()
		m.current = msg.branch
		if branches, err := listBranches(m.root); err == nil {
			m.setBranches(branches)
		} else {
			m.rebuildLabels()
		}
		m.selectByName(selected)
		if msg.newName != "" {
			m.status = "Created and switched to " + msg.newName
		} else {
			m.status = "Switched to " + msg.branch
		}
		m.statusErr = false
		if msg.pullErr != nil {
			m.status += " (the pull failed: " + firstLine(msg.pullErr.Error()) + ")"
			m.statusErr = true
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey acts on a key press.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Ctrl+C always leaves, even while git is working.
	if key == "ctrl+c" {
		m.interrupted = true
		return m, tea.Quit
	}

	// While git is working the picker ignores the keyboard, so a second
	// switch cannot be started on top of the first.
	if m.busy {
		return m, nil
	}

	if m.phase == phaseName {
		return m.handleNameKey(msg, key)
	}

	switch key {
	case "esc":
		// Escape is the exit key.
		return m, tea.Quit
	case "ctrl+s":
		m.toggleSort()
		return m, nil
	case "ctrl+n":
		if len(m.matches) > 0 {
			m.phase = phaseName
			m.nameBuf = ""
			m.nameErr = ""
		}
		return m, nil
	case "enter":
		if len(m.matches) > 0 {
			return m.startSwitch(m.branchAt(m.matches[m.cursor]), "")
		}
		return m, nil
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		m.scrollIntoView()
		return m, nil
	case "down":
		if m.cursor < len(m.matches)-1 {
			m.cursor++
		}
		m.scrollIntoView()
		return m, nil
	case "pgup":
		m.movePage(-1)
		return m, nil
	case "pgdown":
		m.movePage(1)
		return m, nil
	case "backspace":
		if m.query != "" {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.refilter()
			m.scrollIntoView()
		}
		return m, nil
	}

	// Anything else that is a printable character goes into the search
	// buffer.
	if msg.Type == tea.KeyRunes {
		m.query += string(msg.Runes)
		m.refilter()
		m.scrollIntoView()
	}
	return m, nil
}

// handleNameKey acts on a key press while the new branch name is being typed.
func (m model) handleNameKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		// Escape goes back to the list. A second escape leaves.
		m.phase = phaseList
		m.nameBuf = ""
		m.nameErr = ""
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameBuf)
		if name == "" {
			m.nameErr = "a branch name is needed"
			return m, nil
		}
		if err := validBranchName(name); err != nil {
			m.nameErr = err.Error()
			return m, nil
		}
		m.phase = phaseList
		m.nameBuf = ""
		m.nameErr = ""
		return m.startSwitch(m.branchAt(m.matches[m.cursor]), name)
	case "backspace":
		if m.nameBuf != "" {
			runes := []rune(m.nameBuf)
			m.nameBuf = string(runes[:len(runes)-1])
			m.nameErr = ""
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.nameBuf += string(msg.Runes)
		m.nameErr = ""
	}
	return m, nil
}

// startSwitch begins the git work for a picked branch.
//
// The picker stays open while it runs, so the work happens as a command and
// the result comes back as a message.
func (m *model) startSwitch(branch *Branch, newName string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.status = "working..."
	m.statusErr = false
	root := m.root
	action := switchAction{branch: *branch, newName: newName}
	return *m, switchCmd(root, action)
}

// switchAction is the git work to do when a branch is picked.
type switchAction struct {
	branch  Branch
	newName string
}

// switchCmd runs the switch, the pull, and the new branch when there is one.
//
// A branch with no upstream fails its pull, and that is the branch's own
// business rather than a reason to leave the picker, so the pull failure is
// reported on its own.
func switchCmd(root string, action switchAction) tea.Cmd {
	return func() tea.Msg {
		if err := switchBranch(action.branch, root); err != nil {
			return switchDoneMsg{err: err}
		}
		if action.newName == "" {
			if err := pull(root); err != nil {
				return switchDoneMsg{branch: action.branch.Name, pullErr: err}
			}
			return switchDoneMsg{branch: action.branch.Name}
		}
		if err := createBranch(action.newName, root); err != nil {
			return switchDoneMsg{branch: action.branch.Name, err: err}
		}
		return switchDoneMsg{branch: action.newName, newName: action.newName}
	}
}

// selectedName is the branch the pointer is on.
func (m model) selectedName() string {
	if len(m.matches) == 0 {
		return ""
	}
	return m.branchAt(m.matches[m.cursor]).Name
}

// movePage shifts the pointer by a page, stopping at either end.
//
// Paging clamps rather than wrapping even when the list cycles: single-step
// arrows wrapping is handy, but wrapping a screenful at a time throws you
// back to the top of the list for no reason.
func (m *model) movePage(direction int) {
	if len(m.matches) == 0 {
		return
	}
	target := m.cursor + direction*m.pageRows()
	if target < 0 {
		target = 0
	}
	if target > len(m.matches)-1 {
		target = len(m.matches) - 1
	}
	m.cursor = target
	m.scrollIntoView()
}

// toggleSort flips between newest first and alphabetical, keeping the pins
// on top and the pointer where it was.
func (m *model) toggleSort() {
	selected := m.selectedName()
	if m.sortMode == sortTime {
		m.sortMode = sortName
	} else {
		m.sortMode = sortTime
	}
	sortBranches(m.branches, m.sortMode)
	m.rebuildLabels()
	m.refilter()
	m.selectByName(selected)
}

// selectByName puts the pointer back on a named branch.
func (m *model) selectByName(name string) {
	if name == "" {
		return
	}
	for i := range m.matches {
		if m.branchAt(m.matches[i]).Name == name {
			m.cursor = i
			m.scrollIntoView()
			return
		}
	}
}

// View draws the picker.
func (m model) View() string {
	if m.phase == phaseName {
		return m.viewName()
	}

	var b strings.Builder

	// The message line carries the repository folder name.
	message := "Switch to branch:"
	if m.repoName != "" {
		message = m.repoName + "  " + message
	}
	b.WriteString("? " + message + "\n")

	// The search line: the pointer, whatever has been typed, and the live
	// readout of where the pointer sits.
	b.WriteString(pointer + " " + m.query)
	b.WriteString(styleInfo.Render("  ["))
	b.WriteString(styleInfo.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.matches))))
	b.WriteString(styleInfo.Render("]  sort: " + m.sortMode))
	if len(m.matches) != len(m.branches) {
		b.WriteString(styleInfo.Render(fmt.Sprintf("  (%d of %d match)", len(m.matches), len(m.branches))))
	}
	if m.fetchMsg != "" {
		b.WriteString(styleInfo.Render("  " + m.fetchMsg))
	}
	if m.status != "" {
		style := styleGood
		if m.statusErr {
			style = styleError
		}
		b.WriteString(style.Render("  " + m.status))
	}
	b.WriteString("\n")

	// The list.
	//
	// Empty rows are written for the branches that do not fit, so the list
	// always takes the full height and the panel below it stays anchored to
	// the bottom of the screen.
	rows := m.listRows()
	for i := 0; i < rows; i++ {
		index := m.offset + i
		if index >= len(m.matches) {
			b.WriteString("\n")
			continue
		}
		entry := m.matches[index]
		prefix := "  "
		if index == m.cursor {
			prefix = pointer + " "
		}
		b.WriteString(prefix + styleLabel(m.branchAt(entry).Name, m.labels[entry.index], entry.indices, index == m.cursor))
		b.WriteString("\n")
	}

	// The command panel.
	for _, line := range m.panelLines() {
		b.WriteString(line + "\n")
	}

	// The instructions.
	b.WriteString(styleInfo.Render(instruction))
	return b.String()
}

// panelLines draws the command panel: a title rule and the commands that
// will run.
//
// The panel is open on the right and bottom: it is a preview, not a modal,
// and an edge sitting at the terminal margin wraps on some consoles.
func (m model) panelLines() []string {
	width := m.width
	if width < minPanelWidth {
		width = minPanelWidth
	}
	title := "will run"
	head := "+-- "
	tail := width - displayWidth(head) - displayWidth(title) - 1
	if tail < 0 {
		tail = 0
	}
	lines := []string{
		styleBorder.Render(head) + styleTitle.Render(title) + styleBorder.Render(" "+strings.Repeat("-", tail)),
	}

	commands := m.previewCommands()
	for _, command := range commands {
		text := fitWidth(command.text, width-2)
		lines = append(lines, styleBorder.Render("| ")+command.style.Render(padToWidth(text, width-2)))
	}
	return lines
}

// previewCommand is one line of the command panel.
type previewCommand struct {
	text  string
	style lipgloss.Style
}

// previewCommands are the commands the picker will run for the highlighted
// branch.
func (m model) previewCommands() []previewCommand {
	if len(m.matches) == 0 {
		return []previewCommand{{text: "nothing selected", style: styleBorder}}
	}
	branch := *m.branchAt(m.matches[m.cursor])
	commands := []previewCommand{{text: switchCommand(branch), style: styleCmd}}
	if branch.Remote || branch.Upstream != "" {
		commands = append(commands, previewCommand{text: "git pull", style: styleCmd})
	} else {
		commands = append(commands, previewCommand{
			text:  "git pull  (no upstream, will fail)",
			style: styleWarn,
		})
	}
	return commands
}

// viewName draws the new branch name prompt.
func (m model) viewName() string {
	var b strings.Builder

	base := m.branchAt(m.matches[m.cursor]).Name
	message := "New branch from " + base + ":"
	if m.repoName != "" {
		message = m.repoName + "  " + message
	}
	b.WriteString("? " + message + "\n")
	b.WriteString(pointer + " " + m.nameBuf + "\n")

	width := m.width
	if width < minPanelWidth {
		width = minPanelWidth
	}
	title := "will run"
	head := "+-- "
	tail := width - displayWidth(head) - displayWidth(title) - 1
	if tail < 0 {
		tail = 0
	}
	b.WriteString(styleBorder.Render(head) + styleTitle.Render(title) + styleBorder.Render(" "+strings.Repeat("-", tail)) + "\n")

	name := strings.TrimSpace(m.nameBuf)
	if name == "" {
		name = "<name>"
	}
	b.WriteString(styleBorder.Render("| ") + styleCmd.Render(padToWidth("git switch "+base, width-2)) + "\n")
	b.WriteString(styleBorder.Render("| ") + styleCmd.Render(padToWidth("git switch -c "+name, width-2)) + "\n")

	if m.nameErr != "" {
		b.WriteString(styleError.Render(m.nameErr))
	} else {
		b.WriteString(styleInfo.Render("enter to create the branch, esc to go back"))
	}
	return b.String()
}

// styleLabel draws one row of the list.
//
// The first two cells are the current-branch marker, which is coloured on its
// own. The rest is the branch name and the columns, with the characters the
// query matched picked out.
func styleLabel(branchName, label string, indices []int, hovered bool) string {
	runes := []rune(label)
	if len(runes) < 2 {
		return label
	}

	marker := string(runes[:2])
	body := runes[2:]

	matched := make(map[int]bool, len(indices))
	for _, i := range indices {
		if i >= 2 && i < len(runes) {
			matched[i-2] = true
		}
	}

	var b strings.Builder
	if strings.HasPrefix(marker, "*") {
		b.WriteString(styleCurrent.Render(marker))
	} else {
		b.WriteString(marker)
	}

	var run strings.Builder
	runMatched := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		text := run.String()
		switch {
		case runMatched:
			b.WriteString(styleMatch.Render(text))
		case hovered:
			b.WriteString(stylePointer.Render(text))
		default:
			b.WriteString(text)
		}
		run.Reset()
	}
	for i, r := range body {
		isMatch := matched[i]
		if i > 0 && isMatch != runMatched {
			flush()
		}
		runMatched = isMatch
		run.WriteRune(r)
	}
	flush()
	return b.String()
}

// validBranchName rejects the names git itself would refuse.
func validBranchName(name string) error {
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("a branch name cannot start with a dash")
	}
	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("a branch name cannot contain spaces")
	}
	if strings.ContainsAny(name, "~^:?*[\\") {
		return fmt.Errorf("a branch name cannot contain ~ ^ : ? * [ or backslash")
	}
	return nil
}

// firstLine is the first line of a message, for the one line status.
func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}
