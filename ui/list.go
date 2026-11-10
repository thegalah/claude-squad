package ui

import (
	"claude-squad/log"
	"claude-squad/session"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const readyIcon = "● "
const pausedIcon = "⏸ "

var readyStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var addedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var removedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#de613e"))

var pausedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"})

var titleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"})

var selectedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var selectedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

// hoverBackground marks the instance under the mouse pointer: a softer version
// of the selected background.
var hoverBackground = lipgloss.AdaptiveColor{Light: "#eef1f7", Dark: "#2b3040"}

var hoverTitleStyle = titleStyle.Background(hoverBackground)

var hoverDescStyle = listDescStyle.Background(hoverBackground)

// openArrow is drawn at the right of the selected or hovered session's title line, when
// the session can be attached; clicking it attaches.
const openArrow = "→"

// openArrowCols is the space the title line keeps for the arrow and the space after it.
const openArrowCols = 2

var openArrowStyle = lipgloss.NewStyle().Bold(true)

// openArrowHoverStyle marks the arrow while the pointer is on it.
var openArrowHoverStyle = lipgloss.NewStyle().Bold(true).Foreground(highlightColor)

var mainTitle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

var autoYesStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.Color("#1a1a1a"))

type List struct {
	items       []*session.Instance
	selectedIdx int
	// hoverIdx is the item under the mouse pointer, or -1.
	hoverIdx int
	// hoverArrow is set when the pointer is on hoverIdx's open arrow.
	hoverArrow    bool
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int

	// repoName is the name of the repository this cs process is scoped to, shown in the title.
	repoName string
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:    []*session.Instance{},
		renderer: &InstanceRenderer{spinner: spinner},
		repos:    make(map[string]int),
		autoyes:  autoYes,
		hoverIdx: -1,
	}
}

// SetRepoName sets the repository name shown in the list title.
func (l *List) SetRepoName(name string) {
	l.repoName = name
}

// title returns the list title, including the repo name when set. maxWidth is the space
// available for the title text; the repo name is truncated with an ellipsis to fit.
func (l *List) title(maxWidth int) string {
	const base = " Instances "
	if l.repoName == "" {
		return base
	}
	full := fmt.Sprintf(" Instances · %s ", l.repoName)
	if maxWidth <= 0 || runewidth.StringWidth(full) <= maxWidth {
		return full
	}
	// Keep "Instances · " and as much of the repo name as fits.
	prefix := " Instances · "
	room := maxWidth - runewidth.StringWidth(prefix) - 1 // trailing space
	if room < 2 {
		return base
	}
	return prefix + runewidth.Truncate(l.repoName, room, "…") + " "
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	l.width = width
	l.height = height
	l.renderer.setWidth(width)
}

// Width returns the rendered width of the list.
func (l *List) Width() int {
	return l.width
}

// SetSessionPreviewSize sets the height and width for the tmux sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.items {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.items)
}

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner *spinner.Model
	width   int
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = AdjustPreviewWidth(width)
}

// ɹ and ɻ are other options.
const branchIcon = "Ꮧ"

func (r *InstanceRenderer) Render(i *session.Instance, idx int, selected bool, hasMultipleRepos bool) string {
	return r.render(i, idx, selected, false, false, hasMultipleRepos)
}

// CanAttach reports whether i has a running session to attach to, as far as the list can
// tell without asking tmux.
func CanAttach(i *session.Instance) bool {
	return i.Started() && !i.Paused() && i.Status != session.Loading && i.Busy() == session.NotBusy
}

// render is Render with hovered, which highlights an unselected instance under the mouse
// pointer, and onArrow, set when the pointer is on its open arrow.
func (r *InstanceRenderer) render(i *session.Instance, idx int, selected, hovered, onArrow bool, hasMultipleRepos bool) string {
	prefix := fmt.Sprintf(" %d. ", idx)
	if idx >= 10 {
		prefix = prefix[:len(prefix)-1]
	}
	titleS := selectedTitleStyle
	descS := selectedDescStyle
	if !selected && hovered {
		titleS = hoverTitleStyle
		descS = hoverDescStyle
	} else if !selected {
		titleS = titleStyle
		descS = listDescStyle
	}

	// add spinner next to title if it's running
	var join string
	switch {
	case i.Busy() != session.NotBusy:
		join = fmt.Sprintf("%s ", r.spinner.View())
	default:
		switch i.Status {
		case session.Running, session.Loading:
			join = fmt.Sprintf("%s ", r.spinner.View())
		case session.Ready:
			join = readyStyle.Render(readyIcon)
		case session.Paused:
			join = pausedStyle.Render(pausedIcon)
		}
	}

	// Cut the title if it's too long
	titleText := i.Title
	widthAvail := r.width - 3 - openArrowCols - runewidth.StringWidth(prefix) - 1
	if widthAvail > 0 && runewidth.StringWidth(titleText) > widthAvail {
		titleText = runewidth.Truncate(titleText, widthAvail-3, "...")
	}
	arrow := strings.Repeat(" ", openArrowCols)
	if (selected || hovered) && CanAttach(i) {
		style := openArrowStyle
		if onArrow {
			style = openArrowHoverStyle
		}
		arrow = style.Background(titleS.GetBackground()).Render(openArrow) +
			titleS.UnsetPadding().Render(strings.Repeat(" ", openArrowCols-1))
	}
	title := titleS.Render(lipgloss.JoinHorizontal(
		lipgloss.Left,
		lipgloss.Place(r.width-3-openArrowCols, 1, lipgloss.Left, lipgloss.Center, fmt.Sprintf("%s %s", prefix, titleText)),
		" ",
		join,
		arrow,
	))

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	remainingWidth := r.width
	remainingWidth -= runewidth.StringWidth(prefix)
	remainingWidth -= runewidth.StringWidth(branchIcon)
	remainingWidth -= 2 // for the literal " " and "-" in the branchLine format string

	diffWidth := runewidth.StringWidth(addedDiff) + runewidth.StringWidth(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth

	branch := i.Branch
	if i.Started() && hasMultipleRepos {
		repoName, err := i.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name in instance renderer: %v", err)
		} else {
			branch += fmt.Sprintf(" (%s)", repoName)
		}
	}
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	branchWidth := runewidth.StringWidth(branch)
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < branchWidth {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = runewidth.Truncate(branch, remainingWidth-3, "...")
		}
	}
	remainingWidth -= runewidth.StringWidth(branch)

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, spaces, diff)
	if i.Status == session.Loading {
		// The branch isn't interesting yet: say what setup is doing instead, so a slow start
		// (e.g. a long setup script) doesn't look like a hang.
		branchLine = loadingLine(i, len(prefix), r.width)
	} else if op := i.Busy(); op != session.NotBusy {
		branchLine = statusLine(op.Label()+"…", len(prefix), r.width)
	}

	// join title and subtitle
	text := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		descS.Render(branchLine),
	)

	return text
}

// loadingLine renders the second line of an instance that is still being set up, e.g.
// "     running setup.sh · 12s", padded or truncated to width.
func loadingLine(i *session.Instance, indent, width int) string {
	prog := i.StartProgress()
	text := session.StageShortLabel(prog.Stage, i.Program) + "…"
	if !prog.StartedAt.IsZero() {
		text += " " + session.FormatElapsed(time.Since(prog.StartedAt))
	}
	return statusLine(text, indent, width)
}

// statusLine renders text as an instance's second line in place of its branch, indented
// by indent and padded or truncated to width.
func statusLine(text string, indent, width int) string {
	line := strings.Repeat(" ", indent) + " " + text
	if width <= 0 {
		return line
	}
	if runewidth.StringWidth(line) > width {
		return runewidth.Truncate(line, width, "…")
	}
	return line + strings.Repeat(" ", width-runewidth.StringWidth(line))
}

func (l *List) String() string {
	const autoYesText = " auto-yes "

	// Write the title.
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\n")

	// Write title line
	// add padding of 2 because the border on list items adds some extra characters
	titleWidth := AdjustPreviewWidth(l.width) + 2
	if !l.autoyes {
		titleText := l.title(titleWidth - mainTitle.GetHorizontalFrameSize())
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText)))
	} else {
		titleText := l.title(titleWidth/2 - mainTitle.GetHorizontalFrameSize())
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n")
	b.WriteString("\n")

	// Render the list.
	for i, item := range l.renderItems() {
		b.WriteString(item)
		if i != len(l.items)-1 {
			b.WriteString(listItemGap)
		}
	}
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// listItemsOffset is the number of lines String() renders above the first item:
// two blank lines, the title and a blank line.
const listItemsOffset = 4

// listItemGap separates rendered items, leaving listItemGapLines blank lines.
const (
	listItemGap      = "\n\n"
	listItemGapLines = 1
)

func (l *List) renderItems() []string {
	rendered := make([]string, len(l.items))
	for i, item := range l.items {
		hovered := i == l.hoverIdx
		rendered[i] = l.renderer.render(item, i+1, i == l.selectedIdx, hovered, hovered && l.hoverArrow, len(l.repos) > 1)
	}
	return rendered
}

// ItemAt returns the index of the item rendered on line y of String()'s output,
// or -1 if there is none (including the blank lines between items).
func (l *List) ItemAt(y int) int {
	idx, _ := l.itemLineAt(y)
	return idx
}

// itemLineAt returns the item rendered on line y of String()'s output and which of the
// item's lines y is, or -1 if y isn't on an item.
func (l *List) itemLineAt(y int) (idx, line int) {
	if y < listItemsOffset {
		return -1, 0
	}
	top := listItemsOffset
	for i, item := range l.renderItems() {
		height := lipgloss.Height(item)
		if y < top+height {
			return i, y - top
		}
		top += height
		if y < top+listItemGapLines {
			return -1, 0 // the gap after item i
		}
		top += listItemGapLines
	}
	return -1, 0
}

// titleLine is the line of a rendered item that holds its title, below the top padding.
const titleLine = 1

// OpenArrowAt returns the item whose open arrow is at (x, y) of String()'s output, or -1.
// The arrow's target is a little larger than the arrow: the cells beside it, and the
// line below it.
func (l *List) OpenArrowAt(x, y int) int {
	idx, line := l.itemLineAt(y)
	if idx < 0 || (line != titleLine && line != titleLine+1) {
		return -1
	}
	if (idx != l.selectedIdx && idx != l.hoverIdx) || !CanAttach(l.items[idx]) {
		return -1
	}
	// The title line is the style's left padding, the title, a space, the status icon
	// (two cells), then the arrow.
	col := 1 + (l.renderer.width - 3 - openArrowCols) + 1 + 2
	if x < col-1 || x > col+1 {
		return -1
	}
	return idx
}

// SetHover marks the item under the mouse pointer (-1 for none), and whether the pointer
// is on its open arrow.
func (l *List) SetHover(idx int, onArrow bool) {
	l.SetHoverIdx(idx)
	l.hoverArrow = onArrow && l.hoverIdx >= 0
}

// SetHoverIdx marks the item under the mouse pointer; -1 means none.
func (l *List) SetHoverIdx(idx int) {
	if idx < -1 || idx >= len(l.items) {
		idx = -1
	}
	l.hoverIdx = idx
	l.hoverArrow = false
}

// Down selects the next item in the list.
func (l *List) Down() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx < len(l.items)-1 {
		l.selectedIdx++
	} else {
		l.selectedIdx = 0
	}
}

// Kill selects the next item in the list.
func (l *List) Kill() {
	if len(l.items) == 0 {
		return
	}
	targetInstance := l.items[l.selectedIdx]

	// Kill the tmux session
	if err := targetInstance.Kill(); err != nil {
		log.ErrorLog.Printf("could not kill instance: %v", err)
	}
	l.Remove(targetInstance)
}

// Remove takes target out of the list without killing it. The selection stays on the same
// instance, or moves to the previous one if target was selected and last.
func (l *List) Remove(target *session.Instance) {
	idx := -1
	for i, inst := range l.items {
		if inst == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	// Unregister the reponame. Only started instances registered one (see AddInstance).
	if target.Started() {
		if repoName, err := target.RepoName(); err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
		} else {
			l.rmRepo(repoName)
		}
	}

	l.items = append(l.items[:idx], l.items[idx+1:]...)
	if (idx < l.selectedIdx || l.selectedIdx >= len(l.items)) && l.selectedIdx > 0 {
		l.selectedIdx--
	}
}

func (l *List) Attach() (chan struct{}, error) {
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev item in the list.
func (l *List) Up() {
	if len(l.items) == 0 {
		return
	}
	if l.selectedIdx > 0 {
		l.selectedIdx--
	} else {
		l.selectedIdx = len(l.items) - 1
	}
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		log.ErrorLog.Printf("repo %s not found", repo)
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	l.items = append(l.items, instance)
	// The finalizer registers the repo name once the instance is started.
	return func() {
		repoName, err := instance.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
			return
		}

		l.addRepo(repoName)
	}
}

// GetSelectedInstance returns the currently selected instance
func (l *List) GetSelectedInstance() *session.Instance {
	if len(l.items) == 0 {
		return nil
	}
	return l.items[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	if idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
}

// SelectInstance finds and selects the given instance in the list.
func (l *List) SelectInstance(target *session.Instance) {
	for i, inst := range l.items {
		if inst == target {
			l.SetSelectedInstance(i)
			return
		}
	}
}

// MoveUp swaps the selected instance with the one above it.
func (l *List) MoveUp() bool {
	if l.selectedIdx <= 0 || len(l.items) < 2 {
		return false
	}
	l.items[l.selectedIdx], l.items[l.selectedIdx-1] = l.items[l.selectedIdx-1], l.items[l.selectedIdx]
	l.selectedIdx--
	return true
}

// MoveDown swaps the selected instance with the one below it.
func (l *List) MoveDown() bool {
	if l.selectedIdx >= len(l.items)-1 || len(l.items) < 2 {
		return false
	}
	l.items[l.selectedIdx], l.items[l.selectedIdx+1] = l.items[l.selectedIdx+1], l.items[l.selectedIdx]
	l.selectedIdx++
	return true
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}
