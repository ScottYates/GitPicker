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
	"time"

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
type switchDoneMsg struct {
	branch  string
	newName string
	err     error
}

// pullDoneMsg reports the result of a background pull.
type pullDoneMsg struct {
	branch string
	err    error
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

	// pulling is true while a background pull is running. A switch that
	// arrives in that window is held back until the pull is done, because
	// git will not let two commands touch the index at once.
	pulling bool
	pending *switchAction

	// The clock for the timings shown in the status line.
	switchStarted time.Time
	pullStarted   time.Time

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
			m.status = fmt.Sprintf("Created and switched to %s (%.1fs)", msg.newName, time.Since(m.switchStarted).Seconds())
		} else {
			m.status = fmt.Sprintf("Switched to %s (%.1fs)", msg.branch, time.Since(m.switchStarted).Seconds())
		}
		m.statusErr = false
		// The pull is the slow part, so it runs in the background and the
		// picker carries on while it works. A branch with no upstream has
		// nothing to pull.
		if msg.newName == "" && m.hasUpstream(msg.branch) {
			m.pulling = true
			m.pullStarted = time.Now()
			m.status = "Switched to " + msg.branch + ", pulling..."
			return m, pullCmd(m.root, msg.branch)
		}
		return m, nil

	case pullDoneMsg:
		m.pulling = false
		if msg.err != nil {
			m.status = fmt.Sprintf("pull of %s failed: %s", msg.branch, firstLine(msg.err.Error()))
			m.statusErr = true
		} else {
			m.status = fmt.Sprintf("Pulled %s (%.1fs)", msg.branch, time.Since(m.pullStarted).Seconds())
			m.statusErr = false
		}
		// A switch that arrived while the pull was running goes now.
		if m.pending != nil {
			action := *m.pending
			m.pending = nil
			return m.startSwitch(action.branch, action.newName)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// hasUpstream reports whether a branch can be pulled, which is what the
// branch list knows about it.
func (m model) hasUpstream(name string) bool {
	for _, branch := range m.branches {
		if branch.Name == name {
			return branch.Remote || branch.Upstream != ""
		}
	}
	return false
}

// controlKeyName maps a control character to the key it stands for.
//
// A console with virtual terminal input enabled hands over the raw control
// byte with no modifier flag attached, so ctrl+n arrives as an ordinary rune
// holding 0x0e. Bubbletea only turns that into the ctrl+n key when the
// console reports the ctrl modifier, so without this the byte lands in the
// search box and quietly filters the list instead of doing anything. That
// also empties the list, which is why the new branch prompt then refuses to
// open: there is nothing left to branch from.
func controlKeyName(r rune) (string, bool) {
	switch r {
	case 0x00:
		return "ctrl+@", true
	case 0x08:
		return "backspace", true
	case 0x09:
		return "tab", true
	case 0x0a, 0x0d:
		return "enter", true
	case 0x1b:
		return "esc", true
	case 0x1c:
		return "ctrl+\\", true
	case 0x1d:
		return "ctrl+]", true
	case 0x1e:
		return "ctrl+^", true
	case 0x1f:
		return "ctrl+_", true
	case 0x7f:
		return "backspace", true
	}
	if r >= 0x01 && r <= 0x1a {
		// The control characters that stand for a letter, skipping the
		// ones already handled above.
		switch r {
		case 0x09, 0x0a, 0x0d:
			return "", false
		}
		return "ctrl+" + string(rune('a'+r-1)), true
	}
	return "", false
}

// stripControl removes the control characters from text, so that a stray
// byte can never end up in the search box or a branch name.
func stripControl(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
}

// handleKey acts on a key press.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	// Ctrl+C always leaves, even while git is working.
	if key == "ctrl+c" {
		m.interrupted = true
		return m, tea.Quit
	}

	// Escape is the exit key, and it leaves even while git is working.
	// Waiting for a slow command with no way out is the one thing that must
	// never happen.
	if key == "esc" {
		return m, tea.Quit
	}

	if m.phase == phaseName {
		return m.handleNameKey(msg, key)
	}

	switch key {
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
			return m.startOrDefer(m.branchAt(m.matches[m.cursor]), "")
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
	// buffer. A control character never does: it is a key press, not text.
	if msg.Type == tea.KeyRunes {
		text := stripControl(string(msg.Runes))
		if text != "" {
			m.query += text
			m.refilter()
			m.scrollIntoView()
		}
	}
	return m, nil
}

// keyName is the name of a key press, with a control character standing in
// for the combination it represents.
//
// Bubbletea reports ctrl+n as the key ctrl+n when the console tells it the
// ctrl modifier is held, and as an ordinary rune holding 0x0e when it does
// not. The second case is what a terminal in virtual terminal input mode
// does, so the combination is worked out here rather than trusted.
func keyName(msg tea.KeyMsg) string {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		if name, ok := controlKeyName(msg.Runes[0]); ok {
			return name
		}
	}
	return msg.String()
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
		return m.startOrDefer(m.branchAt(m.matches[m.cursor]), name)
	case "backspace":
		if m.nameBuf != "" {
			runes := []rune(m.nameBuf)
			m.nameBuf = string(runes[:len(runes)-1])
			m.nameErr = ""
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		text := stripControl(string(msg.Runes))
		if text != "" {
			m.nameBuf += text
			m.nameErr = ""
		}
	}
	return m, nil
}

// startOrDefer starts a switch, or holds it back while git is already
// working.
//
// Git will not let two commands touch the index at once, so a switch that
// arrives while a pull is running waits its turn rather than racing it.
func (m *model) startOrDefer(branch *Branch, newName string) (tea.Model, tea.Cmd) {
	action := switchAction{branch: *branch, newName: newName}
	if m.gitBusy() {
		m.pending = &action
		m.status = "waiting for the current git command to finish..."
		m.statusErr = false
		return *m, nil
	}
	return m.startSwitch(action.branch, action.newName)
}

// startSwitch begins the git work for a picked branch.
//
// The picker stays open while it runs, so the work happens as a command and
// the result comes back as a message.
func (m *model) startSwitch(branch Branch, newName string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.status = "switching to " + branch.Name + "..."
	m.statusErr = false
	m.switchStarted = time.Now()
	root := m.root
	action := switchAction{branch: branch, newName: newName}
	return *m, switchCmd(root, action)
}

// switchAction is the git work to do when a branch is picked.
type switchAction struct {
	branch  Branch
	newName string
}

// switchCmd runs the switch, and the new branch when there is one.
//
// The pull is not here: it is the slow part, and it runs afterwards in the
// background so that the picker is usable while it waits.
func switchCmd(root string, action switchAction) tea.Cmd {
	return func() tea.Msg {
		if err := switchBranch(action.branch, root); err != nil {
			return switchDoneMsg{err: err}
		}
		if action.newName != "" {
			if err := createBranch(action.newName, root); err != nil {
				return switchDoneMsg{branch: action.branch.Name, err: err}
			}
			return switchDoneMsg{branch: action.newName, newName: action.newName}
		}
		return switchDoneMsg{branch: action.branch.Name}
	}
}

// pullCmd pulls the branch that was just switched to.
func pullCmd(root, branch string) tea.Cmd {
	return func() tea.Msg {
		if err := pull(root); err != nil {
			return pullDoneMsg{branch: branch, err: err}
		}
		return pullDoneMsg{branch: branch}
	}
}

// gitBusy reports whether a git command is running.
func (m model) gitBusy() bool {
	return m.busy || m.pulling
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
