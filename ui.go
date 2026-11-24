package main

// The picker UI.
//
// The layout is the one the Python picker draws:
//
//	row 1        <repo>  Switch to branch:
//	row 2        <query>  [pos/count]  sort: mode
//	rows 3..n    the branch list, one row per branch
//	row n-2      +-- will run ---------------
//	row n-1      | git switch <branch>
//	row n        | git pull
//	row n+1      the instructions, on one row or two
//
// The pointer is only ever used for the row that is picked, so that the
// mark means the same thing everywhere it is seen.
//
// The branch that is checked out is at the top of the list, whatever the
// sort or the query says, because it is the branch the reader is most
// likely to want to see. The pointer starts on it.
//
// The list takes every row the chrome does not need, so it fills the screen
// and reflows when the window is resized.
//
// Switching does not close the picker. The branch is switched, the list is
// rebuilt from what git now reports and the picker carries on, so branches
// can be worked through one after another. Escape leaves.
//
// Ctrl+C starts a commit: the changed files are listed for selection, and
// then the commit message is asked for. The message can be more than one
// line, and ctrl+enter finishes it. Ctrl+P pushes.
//
// The status line carries a spinner while git is working, so a slow command
// does not look like a hang.
//
// The hint at the bottom wraps onto a second line before it is cut short, and
// the question mark opens a panel with every key there is. Ctrl+O shows or
// hides what git last said, and Ctrl+U fetches the newest build from the
// release page and puts it in place of the one that is running.
//
// The panels that show text scroll: the arrows move a line and the page keys
// move a screenful. The position never runs past the end of the text, so a
// panel is never left empty.

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// pointer is the marker in front of the highlighted row.
	//
	// It is the arrow of the console's own character set, so it is drawn
	// the same way in every console font.
	pointer = "\u25ba"

	// The characters the panels are drawn with.
	//
	// They are the line drawing characters of the console, and every
	// console font has them.
	cornerTop    = "\u250c"
	cornerBottom = "\u2514"
	horizontal   = "\u2500"
	vertical     = "\u2502"

	// previewRows is the height of the command panel: a title rule and two
	// command lines.
	previewRows = 3

	// minPanelWidth is the narrowest the panel is allowed to get.
	minPanelWidth = 24

	// chromeRows are the rows the picker spends on everything except the
	// list and the instructions: the message line, the search line and the
	// command panel. The instructions are counted separately, because they
	// take one row or two.
	chromeRows = 2 + previewRows

	filesInstruction = "arrows to move, space to select, enter to continue, esc to cancel"
)

// instruction is the hint at the bottom of the branch list.
//
// The keys are longer than a narrow window, so they are written on two lines
// rather than being cut short.
var instruction = []string{
	"type to filter, arrows/pgup/pgdn, ctrl+s sort, ctrl+n new, ctrl+c commit,",
	"ctrl+p push, ctrl+g clone, ctrl+o output, ctrl+u update, ? help, esc exits",
}

// messageInstruction is the hint at the bottom of the message box.
var messageInstruction = []string{
	"ctrl+enter to commit, enter for a new line, up/down for the lines,",
	"ctrl+p/n for the last messages, esc to go back",
}

// shortInstructions are the hint in fewer words, for a window that is too
// narrow for the whole of it.
var shortInstructions = []string{
	"type to filter, pgup/pgdn, ctrl+s sort, ctrl+n new, ctrl+c commit, ctrl+p push, ctrl+g clone, ctrl+o output, ctrl+u update, ? help, esc exits",
	"pgup/pgdn, ctrl+s sort, ctrl+n new, ctrl+c commit, ctrl+p push, ctrl+g clone, ctrl+o output, ctrl+u update, ? help, esc exits",
	"ctrl+s sort, ctrl+n new, ctrl+c commit, ctrl+p push, ctrl+g clone, ctrl+o output, ctrl+u update, ? help, esc exits",
	"ctrl+c commit, ctrl+p push, ctrl+g clone, ctrl+o output, ctrl+u update, ? help, esc exits",
	"ctrl+o output, ctrl+u update, ? help, esc exits",
	"esc exits",
}

// shortMessageInstructions are the hint of the message box in fewer words.
var shortMessageInstructions = []string{
	"ctrl+enter to commit, enter for a new line, up/down for the lines, ctrl+p/n history, esc to go back",
	"ctrl+enter to commit, enter for a new line, esc to go back",
	"ctrl+enter to commit, esc to go back",
}

// instructionLines is the hint at the bottom of the branch list.
//
// The full list of keys is longer than a narrow window, so it is written on
// two lines instead of being cut short. Only when the window is too narrow
// for both of them are the shorter forms used.
func instructionLines(width int) []string {
	return fitLines(instruction, shortInstructions, width)
}

// messageInstructionLines is the hint at the bottom of the message box.
//
// The message box has more to say than the branch list does, so it has
// shorter forms for a narrow terminal. As in the branch list, the hint is
// written on two lines before it is cut short.
func messageInstructionLines(width int) []string {
	return fitLines(messageInstruction, shortMessageInstructions, width)
}

// fitLines picks the lines of a hint that fit the width.
//
// The full hint is used when every one of its lines fits. Otherwise the
// shorter forms are tried, and the shortest of them that fits is used.
func fitLines(full []string, short []string, width int) []string {
	if width < 20 {
		width = 20
	}
	fits := true
	for _, line := range full {
		if len(line) > width {
			fits = false
			break
		}
	}
	if fits {
		return full
	}
	for _, choice := range short {
		if len(choice) <= width {
			return []string{choice}
		}
	}
	return []string{"esc exits"}
}

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
	styleChecked = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Bold(true)

	// styleCursor is the block drawn over the character the caret is on.
	//
	// The terminal's own cursor is hidden for the lifetime of a bubbletea
	// program, so a text field has to draw its own or the user types blind.
	styleCursor = lipgloss.NewStyle().Reverse(true)
)

// phase is which part of the picker has the keyboard.
type phase int

const (
	phaseList phase = iota
	phaseName
	phaseFiles
	phaseMessage
	phasePushConfirm
	phasePRConfirm
	phaseError
	phaseOutput
	phaseClone
	phaseClipboard
	phaseRemove
	phaseHelp
	phaseUpdateConfirm
)

// What happens once the git output has been closed.
const (
	afterNothing     = ""
	afterPushConfirm = "push"
	afterPRConfirm   = "pr"
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
	output  string
}

// pullDoneMsg reports the result of a background pull.
type pullDoneMsg struct {
	branch string
	err    error
	output string
}

// filesLoadedMsg carries the files git has changes in.
type filesLoadedMsg struct {
	files []FileStatus
	err   error
}

// commitDoneMsg reports the result of a commit.
type commitDoneMsg struct {
	message string
	files   int
	err     error
	output  string
}

// cloneDoneMsg reports the result of a clone, and the folder it went into.
type cloneDoneMsg struct {
	target string
	err    error
	output string
}

// clipboardMsg carries the address that was found on the clipboard.
type clipboardMsg struct{ address string }

// pushDoneMsg reports the result of a push.
type pushDoneMsg struct {
	err    error
	output string

	// prURL is the address of the pull request page, when the push was to
	// GitHub and there was a page to open.
	prURL string
}

// browserDoneMsg reports that a page was opened in the browser.
type browserDoneMsg struct {
	url string
	err error
}

// spinnerTickMsg moves the spinner on.
type spinnerTickMsg struct{}

// spinnerFrames are the frames of the spinner that shows while git works.
//
// The frames are plain ASCII, so they look the same everywhere.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// switchAction is the git work to do when a branch is picked.
type switchAction struct {
	branch  Branch
	newName string
}

// pendingAction is a git action that has to wait for the current one. for the current one.
//
// Git will not let two commands touch the index at once, so an action that
// arrives while another one is running waits its turn rather than racing it.
type pendingAction struct {
	kind    string
	branch  Branch
	newName string
	message string
	paths   []string
}

const (
	actionSwitch = "switch"
	actionCommit = "commit"
	actionPush   = "push"
)

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
	namePos   int
	nameErr   string
	fetchMsg  string
	status    string
	statusErr bool
	busy      bool

	// spinnerFrame is the frame of the spinner that shows while git works.
	spinnerFrame int

	// The file selection for a commit.
	files        []FileStatus
	fileSelected map[string]bool
	fileCursor   int
	fileOffset   int
	fileWidth    int

	// The commit message and the history behind it.
	messageBuf   string
	messagePos   int
	history      []string
	historyIndex int

	// pushChoice is the yes or no answer to the push question.
	pushChoice int

	// prURL is the address of the pull request page, and prChoice is the
	// yes or no answer to the question about opening it.
	prURL    string
	prChoice int

	// The git output of the work in flight, and the commands that wrote
	// it. It is shown when the work is done.
	output       string
	outputTitle  string
	outputScroll int

	// errorScroll is how far the error panel has been scrolled.
	errorScroll int

	// lastOutput is the output of the last thing git said, kept after the
	// panel has been closed so that it can be shown again.
	lastOutput      string
	lastOutputTitle string

	// afterOutput is what happens once the output has been closed.
	afterOutput string

	// The clone address being typed, the address found on the clipboard,
	// and the folder to remove before a fresh clone.
	cloneBuf     string
	clonePos     int
	cloneAddress string
	clipAddress  string
	removeTarget string

	// cloneDir is the directory a clone goes into.
	cloneDir string

	// fatal is the failure that ends the picker, if there is one.
	//
	// It is set when a git command the user asked for fails. The picker
	// shows it and then exits with the exit code git gave.
	fatal *GitError

	// pulling is true while a background pull is running.
	pulling bool
	pending *pendingAction

	// The clock for the timings shown in the status line.
	switchStarted time.Time
	pullStarted   time.Time

	// helpScroll is how far the help panel has been scrolled.
	helpScroll int

	// updating is true while the picker is fetching a newer build.
	updating bool

	// updatedTo is the version that was put in place, and updateChoice is
	// the answer to the question about starting it.
	updatedTo    string
	updateChoice int

	// relaunch is the build that should take the console over when the
	// picker is done with it.
	relaunch string

	fetchCh chan fetchResult
}

// newModel builds the picker state from the branches already on disk.
func newModel(root, repoName, current string, branches []Branch, fetchCh chan fetchResult, history []string) model {
	m := model{
		root:         root,
		repoName:     repoName,
		current:      current,
		sortMode:     sortTime,
		fetchCh:      fetchCh,
		history:      history,
		historyIndex: -1,
		fileSelected: map[string]bool{},
		now:          nowSeconds(),
		width:        80,
		height:       24,
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
	m.matches = hoistCurrent(filterBranches(m.query, m.branches, m.labels), m.branches, m.current)
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
//
// The hint at the bottom takes a row or two, depending on how wide the
// window is, so the list gives up a row when the hint is wrapped.
func (m *model) listRows() int {
	rows := m.height - chromeRows - len(instructionLines(m.width))
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
//
// The clipboard is only looked at when there is no repository, because
// that is the only time a clone from it makes sense.
func (m model) Init() tea.Cmd {
	if m.root == "" {
		return func() tea.Msg {
			return clipboardMsg{address: readClipboard()}
		}
	}
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
			return m.fail(msg.err)
		}
		// The picker stays open, so the list and the current branch marker
		// have to catch up with what git just did.
		selected := m.selectedName()
		m.current = msg.branch
		if branches, err := listBranches(m.root); err == nil {
			m.setBranches(branches)
		} else {
			m.rebuildLabels()
			m.refilter()
		}
		m.selectByName(selected)
		if msg.newName != "" {
			m.status = fmt.Sprintf("Created and switched to %s (%.1fs)", msg.newName, time.Since(m.switchStarted).Seconds())
			m.addOutput("git switch -c "+msg.newName, msg.output)
		} else {
			m.status = fmt.Sprintf("Switched to %s (%.1fs)", msg.branch, time.Since(m.switchStarted).Seconds())
			m.addOutput("git switch "+msg.branch, msg.output)
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
		// The switch and the pull are one thing that was asked for, so
		// they are shown together once the work is done.
		m.showOutput()
		return m.resumePending()

	case pullDoneMsg:
		m.pulling = false
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.status = fmt.Sprintf("Pulled %s (%.1fs)", msg.branch, time.Since(m.pullStarted).Seconds())
		m.statusErr = false
		m.addOutput("git pull", msg.output)
		m.showOutput()
		return m.resumePending()

	case filesLoadedMsg:
		if msg.err != nil {
			return m.fail(msg.err)
		}
		if len(msg.files) == 0 {
			m.phase = phaseList
			m.status = "nothing to commit"
			m.statusErr = false
			return m, nil
		}
		m.files = msg.files
		m.fileSelected = map[string]bool{}
		m.fileCursor = 0
		m.fileOffset = 0
		m.fileWidth = m.fileColumnWidth()
		m.status = ""
		m.statusErr = false
		return m, nil

	case commitDoneMsg:
		m.busy = false
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.status = fmt.Sprintf("Committed %s (%d file%s)", msg.message, msg.files, plural(msg.files))
		m.statusErr = false
		m.addOutput("git commit", msg.output)
		// The output is shown first, and the question about pushing comes
		// after it has been read.
		m.afterOutput = afterPushConfirm
		m.showOutput()
		return m, nil

	case cloneDoneMsg:
		m.busy = false
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.addOutput("git clone", msg.output)
		// The picker moves into the repository that was just cloned, so
		// the branches in it are the ones on offer.
		if err := m.enterRepo(msg.target); err != nil {
			return m.fail(err)
		}
		m.status = "Cloned into " + msg.target
		m.statusErr = false
		m.showOutput()
		return m, nil

	case clipboardMsg:
		// The clipboard is only looked at when there is no repository,
		// because that is the only time a clone makes sense.
		if m.root != "" {
			return m, nil
		}
		address := gitAddress(msg.address)
		if address == "" {
			return m, nil
		}
		m.clipAddress = address
		m.phase = phaseClipboard
		return m, nil

	case pushDoneMsg:
		m.busy = false
		if msg.err != nil {
			return m.fail(msg.err)
		}
		m.status = "Pushed " + m.current
		m.statusErr = false
		m.addOutput("git push", msg.output)
		// A push to GitHub comes with the address of a pull request
		// page, and the user is asked whether to open it. The question
		// comes after the output has been read.
		if msg.prURL != "" {
			m.prURL = msg.prURL
			m.afterOutput = afterPRConfirm
			m.showOutput()
			return m, nil
		}
		m.showOutput()
		return m.resumePending()

	case browserDoneMsg:
		if msg.err != nil {
			m.status = "could not open the browser: " + firstLine(msg.err.Error())
			m.statusErr = true
		} else {
			m.status = "Opened the pull request page"
			m.statusErr = false
		}
		return m, nil

	case updateDoneMsg:
		m.updating = false
		if msg.err != nil {
			m.status = "the update failed: " + firstLine(msg.err.Error())
			m.statusErr = true
			return m, nil
		}
		if !msg.updated {
			if msg.version == version {
				m.status = "already on the newest build (" + msg.version + ")"
			} else {
				m.status = "this build is newer than the published one (" + msg.version + ")"
			}
			m.statusErr = false
			return m, nil
		}
		// The new build is in place, so the user is asked whether it
		// should be started now.
		m.phase = phaseUpdateConfirm
		m.updatedTo = msg.version
		m.updateChoice = 0
		m.status = ""
		m.statusErr = false
		return m, nil

	case spinnerTickMsg:
		// The spinner only goes on while git is working, so it stops by
		// itself as soon as the work is done.
		if !m.gitBusy() {
			return m, nil
		}
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		return m, spinnerTick()

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

// plural is the extra letter a count needs.
func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// addOutput adds what a git command said to the output on screen.
//
// The commands are named as well, because the output of a switch and the
// output of the pull that follows it are shown together and it should be
// clear that both are there.
//
// The text is also kept aside, so that it can be shown again after the
// panel has been closed.
func (m *model) addOutput(title string, output string) {
	if output == "" {
		return
	}
	if m.output != "" {
		m.output += "\n\n"
	}
	m.output += output
	if m.outputTitle == "" {
		m.outputTitle = title
	} else {
		m.outputTitle += ", " + title
	}
	m.lastOutput = m.output
	m.lastOutputTitle = m.outputTitle
}

// showOutput puts the git output on screen.
func (m *model) showOutput() {
	m.phase = phaseOutput
	m.outputScroll = 0
}

// enterRepo moves the picker into a repository.
//
// It is used after a clone, so the branches on offer are the ones in the
// repository that was just cloned.
func (m *model) enterRepo(root string) error {
	branches, err := listBranches(root)
	if err != nil {
		return err
	}
	m.root = root
	m.repoName = repoLabel(root)
	m.current = currentBranch(root)
	m.query = ""
	m.cursor = 0
	m.offset = 0
	m.setBranches(branches)
	return nil
}

// fail records a failure that ends the picker.
//
// The picker shows the error in full and waits for a key, and then it exits
// with the exit code git gave. The action that was held back is dropped,
// because the picker is on its way out.
func (m model) fail(err error) (tea.Model, tea.Cmd) {
	m.fatal = fatalError(err)
	m.phase = phaseError
	m.busy = false
	m.pulling = false
	m.pending = nil
	return m, nil
}

// fatalError turns an error into the GitError the picker exits with.
//
// A git failure carries its own exit code, which is what the process should
// end with. Anything else is a fault in the picker itself, so it exits 1.
func fatalError(err error) *GitError {
	if gitErr, ok := err.(*GitError); ok {
		return gitErr
	}
	message := err.Error()
	return &GitError{Code: 1, Message: message, Detail: message}
}

// resumePending starts the action that was held back, if there is one.
func (m *model) resumePending() (tea.Model, tea.Cmd) {
	if m.pending == nil {
		return *m, nil
	}
	action := *m.pending
	m.pending = nil
	return m.run(action)
}

// run starts a git action.
func (m *model) run(action pendingAction) (tea.Model, tea.Cmd) {
	switch action.kind {
	case actionCommit:
		return m.startCommit(action.message, action.paths)
	case actionPush:
		return m.startPush()
	default:
		return m.startSwitch(action.branch, action.newName)
	}
}

// startOrDefer starts a git action, or holds it back while git is already
// working.
func (m *model) startOrDefer(action pendingAction) (tea.Model, tea.Cmd) {
	if m.gitBusy() {
		m.pending = &action
		m.status = "waiting for the current git command to finish..."
		m.statusErr = false
		return *m, nil
	}
	return m.run(action)
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
	m.clearOutput()
	root := m.root
	action := switchAction{branch: branch, newName: newName}
	return *m, tea.Batch(switchCmd(root, action), spinnerTick())
}

// startCommit commits the selected files.
func (m *model) startCommit(message string, paths []string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.status = "committing..."
	m.statusErr = false
	m.clearOutput()
	root := m.root
	return *m, tea.Batch(commitCmd(root, message, paths), spinnerTick())
}

// startPush pushes the current branch.
func (m *model) startPush() (tea.Model, tea.Cmd) {
	m.busy = true
	m.status = "pushing " + m.current + "..."
	m.statusErr = false
	m.clearOutput()
	root := m.root
	return *m, tea.Batch(pushCmd(root), spinnerTick())
}

// startClone clones a repository into the current directory.
func (m *model) startClone(address string) (tea.Model, tea.Cmd) {
	m.busy = true
	m.status = "cloning " + address + "..."
	m.statusErr = false
	m.clearOutput()
	dir := m.cloneDir
	return *m, tea.Batch(cloneCmd(address, dir), spinnerTick())
}

// clearOutput empties the output on screen, so that each thing that was
// asked for starts with a clean sheet.
func (m *model) clearOutput() {
	m.output = ""
	m.outputTitle = ""
	m.outputScroll = 0
	m.afterOutput = afterNothing
}

// gitBusy reports whether a git command is running.
func (m model) gitBusy() bool {
	return m.busy || m.pulling
}

// spinnerTick moves the spinner on and asks for the next tick.
//
// The tick goes on for as long as git is working, and stops by itself when
// the work is done.
func spinnerTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// switchCmd runs the switch, and the new branch when there is one.
//
// The pull is not here: it is the slow part, and it runs afterwards in the
// background so that the picker is usable while it waits.
func switchCmd(root string, action switchAction) tea.Cmd {
	return func() tea.Msg {
		output, err := switchBranch(action.branch, root)
		if err != nil {
			return switchDoneMsg{err: err, output: output}
		}
		if action.newName != "" {
			created, err := createBranch(action.newName, root)
			if err != nil {
				return switchDoneMsg{branch: action.branch.Name, err: err, output: output + created}
			}
			return switchDoneMsg{branch: action.newName, newName: action.newName, output: output + created}
		}
		return switchDoneMsg{branch: action.branch.Name, output: output}
	}
}

// pullCmd pulls the branch that was just switched to.
func pullCmd(root, branch string) tea.Cmd {
	return func() tea.Msg {
		output, err := pull(root)
		if err != nil {
			return pullDoneMsg{branch: branch, err: err, output: output}
		}
		return pullDoneMsg{branch: branch, output: output}
	}
}

// commitCmd commits the selected files with the message.
func commitCmd(root, message string, paths []string) tea.Cmd {
	return func() tea.Msg {
		output, err := commitFiles(root, message, paths)
		if err != nil {
			return commitDoneMsg{message: message, err: err, output: output}
		}
		return commitDoneMsg{message: message, files: len(paths), output: output}
	}
}

// pushCmd pushes the current branch to its upstream.
func pushCmd(root string) tea.Cmd {
	return func() tea.Msg {
		output, err := push(root)
		if err != nil {
			return pushDoneMsg{err: err, output: output}
		}
		return pushDoneMsg{output: output, prURL: pullRequestURL(root, output)}
	}
}

// cloneCmd clones a repository into a directory.
func cloneCmd(address, dir string) tea.Cmd {
	return func() tea.Msg {
		output, err := cloneRepo(address, dir)
		if err != nil {
			return cloneDoneMsg{err: err, output: output}
		}
		target, _ := cloneTarget(address, dir)
		return cloneDoneMsg{target: target, output: output}
	}
}

// statusFilesCmd lists the files git has changes in.
//
// It runs as a command because git status walks the whole working tree and
// can take a moment on a big repository.
func statusFilesCmd(root string) tea.Cmd {
	return func() tea.Msg {
		files, _, err := statusFiles(root)
		if err != nil {
			return filesLoadedMsg{err: err}
		}
		return filesLoadedMsg{files: files}
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
// byte can never end up in the search box, a branch name or a commit
// message.
func stripControl(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
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

// isSpaceKey reports whether the key press was the space bar.
func isSpaceKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == ' ' {
		return true
	}
	name := msg.String()
	return name == " " || name == "space"
}

// handleKey acts on a key press.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseName:
		return m.handleNameKey(msg)
	case phaseFiles:
		return m.handleFilesKey(msg)
	case phaseMessage:
		return m.handleMessageKey(msg)
	case phasePushConfirm:
		return m.handlePushConfirmKey(msg)
	case phasePRConfirm:
		return m.handlePRConfirmKey(msg)
	case phaseError:
		return m.handleErrorKey(msg)
	case phaseOutput:
		return m.handleOutputKey(msg)
	case phaseClone:
		return m.handleCloneKey(msg)
	case phaseClipboard:
		return m.handleClipboardKey(msg)
	case phaseRemove:
		return m.handleRemoveKey(msg)
	case phaseHelp:
		return m.handleHelpKey(msg)
	case phaseUpdateConfirm:
		return m.handleUpdateConfirmKey(msg)
	}
	return m.handleListKey(msg)
}

// handleErrorKey acts on a key press on the error panel.
//
// The error is on screen to be read, so the arrows and the page keys scroll
// it. Only escape and enter leave the panel. The console pastes the
// clipboard when the mouse is clicked in the window, and a stray click
// should not close the panel while the error is being read.
func (m model) handleErrorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(msg) {
	case "esc", "enter":
		return m, tea.Quit
	case "up":
		m.scrollError(-1)
		return m, nil
	case "down":
		m.scrollError(1)
		return m, nil
	case "pgup":
		m.scrollError(-m.errorRows())
		return m, nil
	case "pgdown":
		m.scrollError(m.errorRows())
		return m, nil
	}
	return m, nil
}

// scrollError moves the error panel by a number of lines.
//
// The position is kept inside the text, so scrolling cannot leave the panel
// empty.
func (m *model) scrollError(delta int) {
	lines := m.errorLines(m.panelWidth() - 2)
	m.errorScroll = clampScroll(m.errorScroll+delta, len(lines), m.errorRows())
}

// handleListKey acts on a key press in the branch list.
func (m model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	// Escape is the exit key, and it leaves even while git is working.
	// Waiting for a slow command with no way out is the one thing that must
	// never happen.
	if key == "esc" {
		return m, tea.Quit
	}

	switch key {
	case "ctrl+c":
		// Ctrl+C starts a commit: the changed files are listed next.
		m.phase = phaseFiles
		m.status = "looking for changes..."
		m.statusErr = false
		return m, statusFilesCmd(m.root)
	case "ctrl+p":
		return m.startOrDefer(pendingAction{kind: actionPush})
	case "ctrl+s":
		m.toggleSort()
		return m, nil
	case "ctrl+n":
		if len(m.matches) > 0 {
			m.phase = phaseName
			m.nameBuf = ""
			m.namePos = 0
			m.nameErr = ""
		}
		return m, nil
	case "ctrl+g":
		// Ctrl+G clones a repository into the current directory.
		m.phase = phaseClone
		m.cloneBuf = ""
		m.clonePos = 0
		m.status = ""
		m.statusErr = false
		return m, nil
	case "?":
		// The question mark opens the help, which lists every key.
		m.phase = phaseHelp
		m.helpScroll = 0
		m.status = ""
		m.statusErr = false
		return m, nil
	case "ctrl+o":
		// Ctrl+O shows what git last said, or hides it if it is on
		// screen already.
		return m.toggleOutput()
	case "ctrl+u":
		// Ctrl+U fetches the newest build and puts it in place of the
		// one that is running.
		if m.updating {
			return m, nil
		}
		m.updating = true
		m.status = "checking for a newer build..."
		m.statusErr = false
		return m, updateCmd(version)
	case "enter":
		if len(m.matches) > 0 {
			return m.startOrDefer(pendingAction{
				kind:   actionSwitch,
				branch: *m.branchAt(m.matches[m.cursor]),
			})
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

// handleNameKey acts on a key press while the new branch name is being typed.
func (m model) handleNameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc":
		// Escape goes back to the list. A second escape leaves.
		m.phase = phaseList
		m.nameBuf = ""
		m.namePos = 0
		m.nameErr = ""
		return m, nil
	case "ctrl+c":
		m.phase = phaseList
		m.nameBuf = ""
		m.namePos = 0
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
		m.namePos = 0
		m.nameErr = ""
		return m.startOrDefer(pendingAction{
			kind:    actionSwitch,
			branch:  *m.branchAt(m.matches[m.cursor]),
			newName: name,
		})
	case "backspace":
		if m.namePos > 0 {
			runes := []rune(m.nameBuf)
			m.nameBuf = string(runes[:m.namePos-1]) + string(runes[m.namePos:])
			m.namePos--
			m.nameErr = ""
		}
		return m, nil
	case "left":
		if m.namePos > 0 {
			m.namePos--
		}
		return m, nil
	case "right":
		if m.namePos < len([]rune(m.nameBuf)) {
			m.namePos++
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes || isSpaceKey(msg) {
		// The space bar arrives as its own key rather than as a rune, so
		// it has to be picked out separately or every space in a commit
		// message would be dropped.
		text := " "
		if msg.Type == tea.KeyRunes {
			text = stripControl(string(msg.Runes))
		}
		if text != "" {
			runes := []rune(m.nameBuf)
			m.nameBuf = string(runes[:m.namePos]) + text + string(runes[m.namePos:])
			m.namePos += len([]rune(text))
			m.nameErr = ""
		}
	}
	return m, nil
}

// handleFilesKey acts on a key press while the files for a commit are being
// chosen.
func (m model) handleFilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc", "ctrl+c":
		m.phase = phaseList
		m.files = nil
		m.fileSelected = map[string]bool{}
		return m, nil
	case "enter":
		paths := m.selectedPaths()
		if len(paths) == 0 {
			m.status = "select at least one file"
			m.statusErr = true
			return m, nil
		}
		m.phase = phaseMessage
		m.messageBuf = ""
		m.messagePos = 0
		m.historyIndex = -1
		m.status = ""
		m.statusErr = false
		return m, nil
	case "up":
		if m.fileCursor > 0 {
			m.fileCursor--
		}
		m.scrollFilesIntoView()
		return m, nil
	case "down":
		if m.fileCursor < len(m.files)-1 {
			m.fileCursor++
		}
		m.scrollFilesIntoView()
		return m, nil
	case "pgup":
		m.moveFilePage(-1)
		return m, nil
	case "pgdown":
		m.moveFilePage(1)
		return m, nil
	}

	if isSpaceKey(msg) && len(m.files) > 0 {
		path := m.files[m.fileCursor].Path
		if m.fileSelected[path] {
			delete(m.fileSelected, path)
		} else {
			m.fileSelected[path] = true
		}
		return m, nil
	}
	return m, nil
}

// handleMessageKey acts on a key press while the commit message is being
// typed.
func (m model) handleMessageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc":
		m.phase = phaseFiles
		m.messageBuf = ""
		m.messagePos = 0
		return m, nil
	case "ctrl+c":
		m.phase = phaseList
		m.messageBuf = ""
		m.messagePos = 0
		m.files = nil
		m.fileSelected = map[string]bool{}
		return m, nil
	case "ctrl+j":
		// Ctrl+Enter finishes the message. A terminal sends it as
		// ctrl+j, because it is the same byte, and the console reader
		// maps it onto the same key, so one name covers both.
		return m.commitMessage()
	case "enter":
		// Enter adds a line to the message, so a message of more than
		// one line can be written.
		m.messageBuf, m.messagePos = insertText(m.messageBuf, m.messagePos, "\n")
		m.historyIndex = -1
		return m, nil
	case "backspace":
		m.messageBuf, m.messagePos = deleteBack(m.messageBuf, m.messagePos)
		m.historyIndex = -1
		return m, nil
	case "left":
		if m.messagePos > 0 {
			m.messagePos--
		}
		return m, nil
	case "right":
		if m.messagePos < len([]rune(m.messageBuf)) {
			m.messagePos++
		}
		return m, nil
	case "up":
		// The caret moves a line up, so a message of more than one line
		// can be worked through.
		m.messagePos = moveCaretLine(m.messageBuf, m.messagePos, -1)
		return m, nil
	case "down":
		m.messagePos = moveCaretLine(m.messageBuf, m.messagePos, 1)
		return m, nil
	case "ctrl+p":
		// The saved messages are walked with ctrl+p and ctrl+n, because
		// the up and down keys move the caret.
		m.recallHistory(1)
		return m, nil
	case "ctrl+n":
		m.recallHistory(-1)
		return m, nil
	}

	if msg.Type == tea.KeyRunes || isSpaceKey(msg) {
		// The space bar arrives as its own key rather than as a rune, so
		// it has to be picked out separately or every space in a commit
		// message would be dropped.
		text := " "
		if msg.Type == tea.KeyRunes {
			text = stripControl(string(msg.Runes))
		}
		if text != "" {
			m.messageBuf, m.messagePos = insertText(m.messageBuf, m.messagePos, text)
			// Typing means the history is no longer being browsed.
			m.historyIndex = -1
		}
	}
	return m, nil
}

// commitMessage commits the selected files with the message that was typed.
//
// The message is remembered before the commit runs, so a commit that fails
// does not lose it.
func (m model) commitMessage() (tea.Model, tea.Cmd) {
	message := strings.TrimSpace(m.messageBuf)
	if message == "" {
		m.status = "a commit message is needed"
		m.statusErr = true
		return m, nil
	}
	paths := m.selectedPaths()
	if len(paths) == 0 {
		m.phase = phaseFiles
		m.status = "select at least one file"
		m.statusErr = true
		return m, nil
	}
	m.history = rememberMessage(m.history, message)
	_ = saveHistory(m.history)
	m.phase = phaseList
	m.messageBuf = ""
	m.messagePos = 0
	m.files = nil
	m.fileSelected = map[string]bool{}
	return m.startOrDefer(pendingAction{
		kind:    actionCommit,
		message: message,
		paths:   paths,
	})
}

// handleOutputKey acts on a key press while the git output is on screen.
//
// The output scrolls, and escape closes it. What happens after that depends
// on what was being done, so it is kept for the closing.
func (m model) handleOutputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc", "ctrl+o":
		return m.closeOutput()
	case "up":
		m.scrollOutput(-1)
		return m, nil
	case "down":
		m.scrollOutput(1)
		return m, nil
	case "pgup":
		m.scrollOutput(-m.outputRows())
		return m, nil
	case "pgdown":
		m.scrollOutput(m.outputRows())
		return m, nil
	}
	return m, nil
}

// scrollOutput moves the output panel by a number of lines.
//
// The position is kept inside the text, so scrolling cannot leave the panel
// empty.
func (m *model) scrollOutput(delta int) {
	lines := m.outputLines(m.panelWidth() - 2)
	m.outputScroll = clampScroll(m.outputScroll+delta, len(lines), m.outputRows())
}

// closeOutput puts the output panel away.
//
// What happens after that depends on what was being done, so it is kept for
// the closing.
func (m model) closeOutput() (tea.Model, tea.Cmd) {
	m.phase = phaseList
	step := m.afterOutput
	m.afterOutput = afterNothing
	m.output = ""
	m.outputTitle = ""
	switch step {
	case afterPushConfirm:
		m.phase = phasePushConfirm
		m.pushChoice = 0
		return m, nil
	case afterPRConfirm:
		m.phase = phasePRConfirm
		m.prChoice = 0
		return m, nil
	}
	return m.resumePending()
}

// toggleOutput shows what git last said, or puts it away when it is on
// screen already.
//
// The text is kept after the panel is closed, so it can be read again
// without running the command once more.
func (m model) toggleOutput() (tea.Model, tea.Cmd) {
	if m.phase == phaseOutput {
		return m.closeOutput()
	}
	if m.lastOutput == "" {
		m.status = "nothing from git yet"
		m.statusErr = false
		return m, nil
	}
	m.output = m.lastOutput
	m.outputTitle = m.lastOutputTitle
	m.showOutput()
	return m, nil
}

// handleCloneKey acts on a key press while the clone address is being
// typed.
func (m model) handleCloneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc":
		m.phase = phaseList
		m.cloneBuf = ""
		m.clonePos = 0
		return m, nil
	case "enter":
		address := strings.TrimSpace(m.cloneBuf)
		if address == "" {
			m.status = "a repository address is needed"
			m.statusErr = true
			return m, nil
		}
		// A folder that is already there has to go before a fresh clone,
		// and that is worth asking about.
		target, ok := cloneTarget(address, m.cloneDir)
		if ok && pathExists(target) {
			m.cloneAddress = address
			m.removeTarget = target
			m.phase = phaseRemove
			return m, nil
		}
		m.cloneBuf = ""
		m.clonePos = 0
		return m.startClone(address)
	case "backspace":
		runes := []rune(m.cloneBuf)
		if m.clonePos > 0 {
			m.cloneBuf = string(runes[:m.clonePos-1]) + string(runes[m.clonePos:])
			m.clonePos--
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes || isSpaceKey(msg) {
		text := " "
		if msg.Type == tea.KeyRunes {
			text = stripControl(string(msg.Runes))
		}
		if text != "" {
			runes := []rune(m.cloneBuf)
			m.cloneBuf = string(runes[:m.clonePos]) + text + string(runes[m.clonePos:])
			m.clonePos += len([]rune(text))
		}
	}
	return m, nil
}

// handleClipboardKey acts on a key press while the clone found on the
// clipboard is being offered.
//
// The pointer starts on yes, because the address on the clipboard is
// usually there to be used.
func (m model) handleClipboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc", "n":
		m.phase = phaseList
		m.clipAddress = ""
		return m, nil
	case "y":
		address := m.clipAddress
		m.clipAddress = ""
		m.phase = phaseList
		target, ok := cloneTarget(address, m.cloneDir)
		if ok && pathExists(target) {
			m.cloneAddress = address
			m.removeTarget = target
			m.phase = phaseRemove
			return m, nil
		}
		return m.startClone(address)
	}
	return m, nil
}

// handleRemoveKey acts on a key press while the question about an existing
// folder is on screen.
//
// The pointer starts on yes, because a fresh clone is what was asked for.
// Saying no goes back to the address, which is still there.
func (m model) handleRemoveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc", "n":
		m.phase = phaseClone
		m.cloneBuf = m.cloneAddress
		m.clonePos = len([]rune(m.cloneBuf))
		m.removeTarget = ""
		return m, nil
	case "y":
		target := m.removeTarget
		m.removeTarget = ""
		m.phase = phaseList
		if err := os.RemoveAll(target); err != nil {
			return m.fail(err)
		}
		return m.startClone(m.cloneAddress)
	}
	return m, nil
}

// recallHistory walks the saved messages.
//
// The history is newest first, so a step of +1 moves towards the older
// messages and a step of -1 moves back towards the newer ones and finally
// to an empty message.
func (m *model) recallHistory(step int) {
	if len(m.history) == 0 {
		return
	}
	index := m.historyIndex + step
	if index < -1 {
		index = -1
	}
	if index >= len(m.history) {
		index = len(m.history) - 1
	}
	m.historyIndex = index
	if index < 0 {
		m.messageBuf = ""
		m.messagePos = 0
		return
	}
	m.messageBuf = m.history[index]
	m.messagePos = len([]rune(m.messageBuf))
}

// handlePushConfirmKey acts on a key press while the push question is on
// screen.
//
// The pointer starts on yes, because the usual reason for asking is that the
// commit should go out. Escape and n leave it alone.
func (m model) handlePushConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc", "n":
		m.phase = phaseList
		m.pushChoice = 0
		return m.resumePending()
	case "y":
		m.phase = phaseList
		m.pushChoice = 0
		return m.startOrDefer(pendingAction{kind: actionPush})
	case "up":
		if m.pushChoice > 0 {
			m.pushChoice--
		}
		return m, nil
	case "down":
		if m.pushChoice < 1 {
			m.pushChoice++
		}
		return m, nil
	case "enter":
		push := m.pushChoice == 0
		m.phase = phaseList
		m.pushChoice = 0
		if push {
			return m.startOrDefer(pendingAction{kind: actionPush})
		}
		return m.resumePending()
	}
	return m, nil
}

// handlePRConfirmKey acts on a key press while the pull request question is
// on screen.
//
// The pointer starts on yes, because the page is the point of the push.
// Escape and n leave it alone.
func (m model) handlePRConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyName(msg)

	switch key {
	case "esc", "n":
		m.phase = phaseList
		m.prURL = ""
		m.prChoice = 0
		return m.resumePending()
	case "y":
		address := m.prURL
		m.phase = phaseList
		m.prURL = ""
		m.prChoice = 0
		m.status = "opening the pull request page..."
		m.statusErr = false
		return m, openURLCmd(address)
	case "up":
		if m.prChoice > 0 {
			m.prChoice--
		}
		return m, nil
	case "down":
		if m.prChoice < 1 {
			m.prChoice++
		}
		return m, nil
	case "enter":
		open := m.prChoice == 0
		address := m.prURL
		m.phase = phaseList
		m.prURL = ""
		m.prChoice = 0
		if open {
			m.status = "opening the pull request page..."
			m.statusErr = false
			return m, openURLCmd(address)
		}
		return m.resumePending()
	}
	return m, nil
}

// openURLCmd opens an address in the default browser.
//
// It runs as a command so the picker does not freeze while the browser
// starts.
func openURLCmd(address string) tea.Cmd {
	return func() tea.Msg {
		if err := openBrowser(address); err != nil {
			return browserDoneMsg{url: address, err: err}
		}
		return browserDoneMsg{url: address}
	}
}

// outputRows is how many lines of output fit on the screen.
//
// The panel has a heading, a blank line under it, a rule at the bottom and
// the hint below that.
func (m model) outputRows() int {
	rows := m.height - 4
	if rows < 1 {
		rows = 1
	}
	return rows
}

// outputLines is the git output, wrapped to the width.
func (m model) outputLines(width int) []string {
	var lines []string
	for _, line := range strings.Split(m.output, "\n") {
		lines = append(lines, wrapText(line, width)...)
	}
	return lines
}

// selectedPaths are the files that are ticked.
func (m model) selectedPaths() []string {
	var paths []string
	for _, file := range m.files {
		if m.fileSelected[file.Path] {
			paths = append(paths, file.Path)
		}
	}
	return paths
}

// fileColumnWidth is the width of the path column, so the file names line
// up down the list.
func (m model) fileColumnWidth() int {
	width := 0
	for _, file := range m.files {
		if w := displayWidth(file.Path); w > width {
			width = w
		}
	}
	limit := m.width - 12
	if limit < 8 {
		limit = 8
	}
	if width > limit {
		width = limit
	}
	return width
}

// fileRows is how many rows the file list may occupy.
func (m model) fileRows() int {
	rows := m.height - 3
	if rows < 1 {
		rows = 1
	}
	return rows
}

// scrollFilesIntoView moves the file window so the pointer is visible.
func (m *model) scrollFilesIntoView() {
	rows := m.fileRows()
	if m.fileCursor < m.fileOffset {
		m.fileOffset = m.fileCursor
	}
	if m.fileCursor >= m.fileOffset+rows {
		m.fileOffset = m.fileCursor - rows + 1
	}
	if m.fileOffset < 0 {
		m.fileOffset = 0
	}
}

// moveFilePage shifts the file pointer by a page, stopping at either end.
func (m *model) moveFilePage(direction int) {
	if len(m.files) == 0 {
		return
	}
	rows := m.fileRows() - 1
	if rows < 1 {
		rows = 1
	}
	target := m.fileCursor + direction*rows
	if target < 0 {
		target = 0
	}
	if target > len(m.files)-1 {
		target = len(m.files) - 1
	}
	m.fileCursor = target
	m.scrollFilesIntoView()
}

// View draws the picker.
func (m model) View() string {
	switch m.phase {
	case phaseName:
		return m.viewName()
	case phaseFiles:
		return m.viewFiles()
	case phaseMessage:
		return m.viewMessage()
	case phasePushConfirm:
		return m.viewPushConfirm()
	case phasePRConfirm:
		return m.viewPRConfirm()
	case phaseError:
		return m.viewError()
	case phaseOutput:
		return m.viewOutput()
	case phaseHelp:
		return m.viewHelp()
	case phaseUpdateConfirm:
		return m.viewUpdateConfirm()
	case phaseClone:
		return m.viewClone()
	case phaseClipboard:
		return m.viewClipboard()
	case phaseRemove:
		return m.viewRemove()
	}
	return m.viewList()
}

// viewPushConfirm asks whether the commit should be pushed.
func (m model) viewPushConfirm() string {
	var b strings.Builder

	question := "Push " + m.current + "?"
	if upstream := m.upstreamFor(m.current); upstream != "" {
		question = "Push " + m.current + " to " + upstream + "?"
	}
	if m.repoName != "" {
		question = m.repoName + "  " + question
	}
	b.WriteString("  " + question + "\n")

	yes := "  yes"
	no := "  no"
	if m.pushChoice == 0 {
		yes = pointer + " yes"
	} else {
		no = pointer + " no"
	}
	b.WriteString(yes + "\n")
	b.WriteString(no + "\n")
	b.WriteString(styleInfo.Render("y or n, enter to choose, esc to skip"))
	return b.String()
}

// viewPRConfirm asks whether the pull request page should be opened.
func (m model) viewPRConfirm() string {
	var b strings.Builder

	question := "Open the pull request page?"
	if m.repoName != "" {
		question = m.repoName + "  " + question
	}
	b.WriteString("  " + question + "\n")

	yes := "  yes"
	no := "  no"
	if m.prChoice == 0 {
		yes = pointer + " yes"
	} else {
		no = pointer + " no"
	}
	b.WriteString(yes + "\n")
	b.WriteString(no + "\n")

	// The address is shown so it is clear where the browser is going.
	width := m.width
	if width < 20 {
		width = 20
	}
	b.WriteString(fitWidth(m.prURL, width) + "\n")

	b.WriteString(styleInfo.Render("y or n, enter to choose, esc to skip"))
	return b.String()
}

// viewOutput draws what git said.
//
// The output is usually longer than the screen, so it scrolls, and the
// position is shown at the bottom.
func (m model) viewOutput() string {
	var b strings.Builder

	header := m.outputTitle
	if header == "" {
		header = "git output"
	}
	if m.repoName != "" {
		header = m.repoName + "  " + header
	}
	b.WriteString("  " + fitWidth(header, m.panelWidth()) + "\n\n")

	width := m.panelWidth()
	lines := m.outputLines(width - 2)
	rows := m.outputRows()

	start := clampScroll(m.outputScroll, len(lines), rows)
	end := start + rows
	if end > len(lines) {
		end = len(lines)
	}
	for _, line := range lines[start:end] {
		b.WriteString(styleBorder.Render(vertical+" ") + padToWidth(line, width-2) + "\n")
	}
	b.WriteString(panelBottom(width) + "\n")

	position := scrollPosition(start, len(lines), rows)
	b.WriteString(styleInfo.Render("esc closes the output, arrows scroll (" + position + ")"))
	return b.String()
}

// viewClone draws the prompt for the address of a repository to clone.
func (m model) viewClone() string {
	var b strings.Builder

	message := "Clone:"
	if m.repoName != "" {
		message = m.repoName + "  " + message
	}
	b.WriteString("  " + message + "\n")
	b.WriteString(pointer + " " + withCursor(m.cloneBuf, m.clonePos) + "\n")

	// The command that will run is shown, so there is no doubt about
	// what is about to happen.
	width := m.panelWidth()
	b.WriteString(panelRule(width, "will run") + "\n")

	command := "git clone " + strings.TrimSpace(m.cloneBuf)
	if strings.TrimSpace(m.cloneBuf) == "" {
		command = "git clone <address>"
	}
	b.WriteString(styleBorder.Render(vertical+" ") + styleCmd.Render(padToWidth(fitWidth(command, width-2), width-2)) + "\n")

	b.WriteString(styleInfo.Render("enter to clone, esc to cancel"))
	return b.String()
}

// viewClipboard asks whether the repository on the clipboard should be
// cloned.
func (m model) viewClipboard() string {
	var b strings.Builder

	question := "Clone " + repoFolder(m.clipAddress) + "?"
	if m.repoName != "" {
		question = m.repoName + "  " + question
	}
	b.WriteString("  " + question + "\n")
	b.WriteString(pointer + " yes\n")
	b.WriteString("  no\n")

	width := m.panelWidth()
	b.WriteString(fitWidth("git clone "+m.clipAddress, width) + "\n")

	b.WriteString(styleInfo.Render("y or n, enter to choose, esc to skip"))
	return b.String()
}

// viewRemove asks whether a folder that is in the way should be removed.
func (m model) viewRemove() string {
	var b strings.Builder

	question := "Remove " + m.removeTarget + " and clone again?"
	if m.repoName != "" {
		question = m.repoName + "  " + question
	}
	b.WriteString("  " + question + "\n")
	b.WriteString(pointer + " yes\n")
	b.WriteString("  no\n")

	width := m.panelWidth()
	b.WriteString(fitWidth("the folder is removed and the clone is made again", width) + "\n")

	b.WriteString(styleInfo.Render("y or n, enter to choose, esc to skip"))
	return b.String()
}

// panelWidth is the width the panels are drawn at.
func (m model) panelWidth() int {
	width := m.width
	if width < minPanelWidth {
		width = minPanelWidth
	}
	return width
}

// upstreamFor is the remote branch a local branch tracks, or "" when it has
// none.
func (m model) upstreamFor(name string) string {
	for _, branch := range m.branches {
		if branch.Name == name {
			return branch.Upstream
		}
	}
	return ""
}

// viewError draws a fatal error.
//
// The same text is written to stderr when the program exits, so this is for
// reading rather than for catching. The error is usually longer than the
// screen, so it scrolls.
func (m model) viewError() string {
	var b strings.Builder

	head := "git failed"
	if m.fatal != nil {
		head = m.fatal.Message
	}
	if m.repoName != "" {
		head = m.repoName + "  " + head
	}
	b.WriteString(styleError.Render("! "+head) + "\n\n")

	width := m.width
	if width < minPanelWidth {
		width = minPanelWidth
	}

	b.WriteString(panelRule(width, "error") + "\n")

	lines := m.errorLines(width - 2)
	rows := m.errorRows()
	start := clampScroll(m.errorScroll, len(lines), rows)
	end := start + rows
	if end > len(lines) {
		end = len(lines)
	}
	for _, line := range lines[start:end] {
		b.WriteString(styleBorder.Render(vertical+" ") + padToWidth(line, width-2) + "\n")
	}
	b.WriteString(panelBottom(width) + "\n")

	position := scrollPosition(start, len(lines), rows)
	b.WriteString(styleInfo.Render("esc exits, arrows scroll (" + position + ")"))
	return b.String()
}

// errorRows is how many lines of the error fit on the screen.
//
// The panel has a heading, a blank line, a rule above and below the text
// and the hint under it.
func (m model) errorRows() int {
	rows := m.height - 5
	if rows < 1 {
		rows = 1
	}
	return rows
}

// errorLines are the lines of the error.
//
// A long line is wrapped rather than cut, so the whole error is on screen.
// The panel scrolls through them, so nothing is left out.
func (m model) errorLines(width int) []string {
	if width < 8 {
		width = 8
	}

	detail := ""
	if m.fatal != nil {
		detail = m.fatal.Detail
	}
	if detail == "" {
		detail = "no further detail"
	}

	var lines []string
	for _, line := range strings.Split(detail, "\n") {
		lines = append(lines, wrapText(line, width)...)
	}
	return lines
}

// wrapText breaks a line into pieces that fit the width.
//
// The cut is on cell boundaries rather than on word boundaries, so the
// indented output git writes keeps its shape.
func wrapText(text string, width int) []string {
	if displayWidth(text) <= width {
		return []string{text}
	}

	var lines []string
	var current strings.Builder
	used := 0
	for _, r := range text {
		size := runeWidth(r)
		if used+size > width {
			lines = append(lines, current.String())
			current.Reset()
			used = 0
		}
		current.WriteRune(r)
		used += size
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

// viewList draws the branch list.
func (m model) viewList() string {
	var b strings.Builder

	// The message line carries the repository folder name.
	message := "Switch to branch:"
	if m.root == "" {
		// There is no repository, so there is no branch to switch to.
		message = "No repository here:"
	}
	if m.repoName != "" {
		message = m.repoName + "  " + message
	}
	b.WriteString("  " + message + "\n")

	// The search line: whatever has been typed, and the live readout of
	// where the pointer sits.
	b.WriteString("  " + m.query)
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
		text := m.status
		if m.gitBusy() {
			// The spinner shows that git is still working, so the wait
			// does not look like a hang.
			text = spinnerFrames[m.spinnerFrame] + " " + text
		}
		b.WriteString(style.Render("  " + text))
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
			// With no repository there is nothing to list, so the way
			// to get one is said instead.
			if i == 0 && m.root == "" {
				b.WriteString("  " + styleInfo.Render("ctrl+g clones a repository into this folder") + "\n")
				continue
			}
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
	//
	// The hint is wrapped onto a second line rather than being cut short,
	// so the list gives up a row to it.
	hints := instructionLines(m.width)
	for i, hint := range hints {
		if i == len(hints)-1 {
			b.WriteString(styleInfo.Render(hint))
			break
		}
		b.WriteString(styleInfo.Render(hint) + "\n")
	}
	return b.String()
}

// viewFiles draws the file selection for a commit.
func (m model) viewFiles() string {
	var b strings.Builder

	message := "Select files to commit:"
	if m.repoName != "" {
		message = m.repoName + "  " + message
	}
	b.WriteString("  " + message + "\n")

	b.WriteString("  ")
	b.WriteString(styleInfo.Render("["))
	b.WriteString(styleInfo.Render(fmt.Sprintf("%d/%d", m.fileCursor+1, len(m.files))))
	b.WriteString(styleInfo.Render(fmt.Sprintf("]  %d selected", len(m.selectedPaths()))))
	if m.status != "" {
		style := styleGood
		if m.statusErr {
			style = styleError
		}
		b.WriteString(style.Render("  " + m.status))
	}
	b.WriteString("\n")

	rows := m.fileRows()
	for i := 0; i < rows; i++ {
		index := m.fileOffset + i
		if index >= len(m.files) {
			b.WriteString("\n")
			continue
		}
		file := m.files[index]
		prefix := "  "
		if index == m.fileCursor {
			prefix = pointer + " "
		}
		b.WriteString(prefix + m.fileLabel(file) + "\n")
	}

	b.WriteString(styleInfo.Render(filesInstruction))
	return b.String()
}

// fileLabel renders one row of the file list.
func (m model) fileLabel(file FileStatus) string {
	mark := "[ ]"
	if m.fileSelected[file.Path] {
		mark = styleChecked.Render("[x]")
	}
	return mark + " " + file.Code + " " + padToWidth(file.Path, m.fileWidth)
}

// viewMessage draws the commit message box.
//
// The message can be more than one line, so the box holds as many lines as
// there is room for and scrolls to keep the caret on screen.
func (m model) viewMessage() string {
	var b strings.Builder

	message := "Commit message:"
	if m.repoName != "" {
		message = m.repoName + "  " + message
	}
	b.WriteString("  " + message + "\n")

	hints := messageInstructionLines(m.width)
	if m.historyIndex >= 0 {
		hints = []string{fmt.Sprintf("ctrl+enter to commit, %d of %d saved messages", m.historyIndex+1, len(m.history))}
	}

	lines := messageLines(m.messageBuf)
	place := caretPlace(m.messageBuf, m.messagePos)
	rows := m.messageRows(len(hints))
	start := messageScroll(lines, place.line, rows)

	for i := start; i < start+rows && i < len(lines); i++ {
		text := lines[i]
		if i == place.line {
			text = withCursor(text, place.column)
		}
		b.WriteString(pointer + " " + text + "\n")
	}

	for i, hint := range hints {
		if i == len(hints)-1 {
			b.WriteString(styleInfo.Render(hint))
			break
		}
		b.WriteString(styleInfo.Render(hint) + "\n")
	}
	return b.String()
}

// messageRows is how many lines the message box may show.
//
// The box shares the screen with the heading and the hint, so the message
// gets what is left of it. The hint takes a row or two.
func (m model) messageRows(hintRows int) int {
	rows := m.height - 1 - hintRows
	if rows < 1 {
		rows = 1
	}
	return rows
}

// withCursor renders text with a block cursor over the character at the
// given position.
//
// The terminal's own cursor is hidden for the lifetime of a bubbletea
// program, so a text field has to draw its own or the user types blind.
func withCursor(text string, pos int) string {
	runes := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	if pos == len(runes) {
		// The caret is past the end, so it sits on a space.
		return string(runes) + styleCursor.Render(" ")
	}
	return string(runes[:pos]) + styleCursor.Render(string(runes[pos])) + string(runes[pos+1:])
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
	lines := []string{panelRule(width, "will run")}

	commands := m.previewCommands()
	for _, command := range commands {
		text := fitWidth(command.text, width-2)
		lines = append(lines, styleBorder.Render(vertical+" ")+command.style.Render(padToWidth(text, width-2)))
	}
	return lines
}

// panelRule draws the top rule of a panel, with the title in it.
func panelRule(width int, title string) string {
	head := cornerTop + horizontal + " "
	tail := width - displayWidth(head) - displayWidth(title) - 1
	if tail < 0 {
		tail = 0
	}
	return styleBorder.Render(head) + styleTitle.Render(title) +
		styleBorder.Render(" "+strings.Repeat(horizontal, tail))
}

// panelBottom draws the rule under a panel.
func panelBottom(width int) string {
	return styleBorder.Render(cornerBottom + strings.Repeat(horizontal, width-1))
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
	b.WriteString("  " + message + "\n")
	b.WriteString(pointer + " " + withCursor(m.nameBuf, m.namePos) + "\n")

	width := m.width
	if width < minPanelWidth {
		width = minPanelWidth
	}
	b.WriteString(panelRule(width, "will run") + "\n")

	name := strings.TrimSpace(m.nameBuf)
	if name == "" {
		name = "<name>"
	}
	b.WriteString(styleBorder.Render(vertical+" ") + styleCmd.Render(padToWidth("git switch "+base, width-2)) + "\n")
	b.WriteString(styleBorder.Render(vertical+" ") + styleCmd.Render(padToWidth("git switch -c "+name, width-2)) + "\n")

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
