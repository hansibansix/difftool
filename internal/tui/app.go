package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Package tui is the difftool application: the side-by-side model, the
// directory tree, settings, themes, key bindings and the agent notes UI.

// closeFileMsg is emitted by an embedded file diff view when the user
// closes it: focus goes back to the tree pane.
type closeFileMsg struct{}

// switchFileMsg asks the app to open the next/previous file of the dir list.
type switchFileMsg struct{ delta int }

// minSplitWidth is the terminal width from which dir mode shows the tree
// and the diff side by side; below it the panes alternate full-screen.
const minSplitWidth = 90

type app struct {
	dir  *dirModel
	file *model
	w, h int

	// dir mode: which pane has focus, which entry the diff pane shows, and
	// what to say in the diff pane when there is nothing to show
	focusDiff bool
	openedRel string
	note      string

	menuOpen bool
	menuSel  int
	helpOpen bool
	helpTop  int

	// ignore pattern editor inside the settings menu
	ignEdit, ignInput bool
	ignSel            int
	ignText           string
	// ignore regex input inside the settings menu
	reInput       bool
	reText, reErr string
}

func (a *app) Init() tea.Cmd { return pollTick() }

func (a *app) split() bool { return a.dir != nil && cfg.ShowTree && a.w >= minSplitWidth }

func (a *app) treeW() int { return clamp(a.w/3, 30, 50) }

// layout sizes the panes for the current terminal and mode.
func (a *app) layout() {
	if a.dir == nil {
		if a.file != nil {
			a.file.w, a.file.h = a.w, a.h
			a.file.clampScroll()
		}
		return
	}
	a.dir.w, a.dir.h = a.w, a.h
	if a.split() {
		a.dir.w = a.treeW()
	}
	a.dir.ensureVisible()
	if a.file != nil {
		a.file.w, a.file.h = a.w, a.h
		if a.split() {
			a.file.w = a.w - a.treeW() - 1
		}
		a.file.clampScroll()
	}
}

func (a *app) fileDirty() bool { return a.file != nil && a.file.dirty() }

func (a *app) unsavedStatus() string {
	return "unsaved changes in " + filepath.Base(a.openedRel) + " — save or undo first"
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.layout()
		if a.dir != nil && a.file == nil && a.note == "" {
			a.openSelected()
			// start in the diff pane; the tree is one tab away
			a.focusDiff = a.split() && a.file != nil
		}
		return a, nil
	case editDoneMsg:
		a.file.reload(msg.err)
		if a.dir != nil {
			a.dir.refreshSelected()
		}
		return a, nil
	case pollMsg:
		if sidecar.Path != "" && sidecar.Reload() && a.file != nil {
			a.file.loadFileNotes()
			a.file.recompute()
			a.file.status = "notes reloaded"
		}
		if a.file != nil && a.file.checkDisk() && a.dir != nil {
			a.dir.refreshSelected()
		}
		return a, pollTick()
	case closeFileMsg:
		if a.dir == nil {
			return a, tea.Quit
		}
		a.focusDiff = false
		a.dir.refreshSelected()
		return a, nil
	case switchFileMsg:
		switch {
		case a.dir == nil:
			a.file.status = "next/prev file needs directory mode"
		case a.fileDirty():
			a.file.status = "unsaved changes — s to save or u to undo first"
		default:
			a.dir.refreshSelected()
			a.dir.move(msg.delta)
			a.openSelected()
		}
		return a, nil
	}
	if a.helpOpen {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "j", "down":
				a.helpTop++
			case "k", "up":
				a.helpTop--
			case "ctrl+d":
				a.helpTop += (a.h - 2) / 2
			case "ctrl+u":
				a.helpTop -= (a.h - 2) / 2
			default:
				a.helpOpen, a.helpTop = false, 0
			}
		}
		return a, nil
	}
	if a.menuOpen {
		return a.updateMenu(msg)
	}
	if k, ok := msg.(tea.KeyMsg); ok && !a.inputActive() {
		switch keys.global.action(k.String()) {
		case "settings":
			a.menuOpen = true
			return a, nil
		case "help":
			a.helpOpen = true
			return a, nil
		case "tree-toggle":
			if a.dir != nil {
				a.toggleTree()
				return a, nil
			}
		case "ignore-add":
			// quick "ignore this": open the pattern editor prefilled with the
			// selected file's name
			if a.dir != nil && !a.focusDiff {
				a.menuOpen, a.ignEdit, a.ignInput = true, true, true
				a.ignText = ""
				if e := a.dir.selected(); e != nil {
					a.ignText = filepath.Base(e.rel)
				}
				return a, nil
			}
		}
	}
	if a.dir == nil {
		cmd := a.file.update(msg)
		return a, cmd
	}
	// mouse events go to the pane under the pointer; a click also focuses it
	if mm, ok := msg.(tea.MouseMsg); ok && a.split() {
		toDiff := mm.X > a.treeW() && a.file != nil
		if mm.Action == tea.MouseActionPress && mm.Button == tea.MouseButtonLeft {
			a.focusDiff = toDiff
		}
		if toDiff {
			mm.X -= a.treeW() + 1
			return a.updateDiff(mm)
		}
		return a.updateTree(mm)
	}
	if a.focusDiff && a.file != nil {
		return a.updateDiff(msg)
	}
	return a.updateTree(msg)
}

// updateDiff routes a message to the diff pane in dir mode.
func (a *app) updateDiff(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && keys.file.action(k.String()) == "tree" && !a.file.searchInput && !a.file.visual {
		a.focusDiff = false
		a.dir.refreshSelected()
		return a, nil
	}
	wasDirty := a.fileDirty()
	cmd := a.file.update(msg)
	if wasDirty && !a.fileDirty() {
		a.dir.refreshSelected() // saved (or undone to the saved state): re-read the status
	}
	return a, cmd
}

// updateTree routes a message to the tree pane; a changed selection opens
// the new file in the diff pane unless the current one has unsaved changes.
func (a *app) updateTree(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && !a.dir.filterInput {
		switch keys.dir.action(k.String()) {
		case "open":
			if a.file == nil {
				a.openSelected()
			}
			if a.file != nil {
				a.focusDiff = true
			} else if a.note != "" {
				a.dir.status = a.note
			}
			return a, nil
		case "copy-left", "copy-right", "undo":
			if a.fileDirty() {
				a.dir.status = a.unsavedStatus()
				return a, nil
			}
			cmd := a.dir.update(msg)
			a.openSelected() // copy/undo changed the file on disk: show the new state
			return a, cmd
		}
	}
	prev := a.dir.selected()
	prevRel, prevStatus := "", stSame
	if prev != nil {
		prevRel, prevStatus = prev.rel, prev.status
	}
	cmd := a.dir.update(msg)
	cur := a.dir.selected()
	switch {
	case cur == nil:
	case cur.rel != a.openedRel && a.fileDirty():
		a.dir.status = a.unsavedStatus()
		a.dir.selectRel(prevRel)
	case cur.rel != a.openedRel, cur.rel == prevRel && cur.status != prevStatus:
		a.openSelected() // new selection, or the file changed on disk (e.g. confirmed delete)
	}
	return a, cmd
}

// inputActive reports whether a text input (search or filter) is capturing
// keys, so app-level shortcuts must stay out of the way.
func (a *app) inputActive() bool {
	if a.dir == nil {
		return a.file.searchInput || a.file.noteInput
	}
	if a.focusDiff && a.file != nil {
		return a.file.searchInput || a.file.noteInput
	}
	return a.dir.filterInput
}

func (a *app) View() string {
	if a.helpOpen {
		return a.helpView()
	}
	if a.menuOpen {
		return a.settingsView()
	}
	if a.dir == nil {
		return a.file.view(false)
	}
	dirtyRel := ""
	if a.fileDirty() {
		dirtyRel = a.openedRel
	}
	if !a.split() {
		if a.focusDiff && a.file != nil {
			return a.file.view(true)
		}
		return a.dir.view(true, dirtyRel)
	}
	right := a.placeholder()
	if a.file != nil {
		right = a.file.view(a.focusDiff)
	}
	sep := strings.TrimSuffix(strings.Repeat(styleSep.Render("│")+"\n", a.h), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, a.dir.view(!a.focusDiff, dirtyRel), sep, right)
}

// placeholder fills the diff pane when no file is shown.
func (a *app) placeholder() string {
	w := a.w - a.treeW() - 1
	var b strings.Builder
	b.WriteString(barPad(styleBar.Render(" ")+styleHeaderDim.Render("no file"), w) + "\n")
	for i := 0; i < max(1, a.h-2); i++ {
		if i == 1 {
			b.WriteString(styleGutter.Render("  "+a.note) + "\n")
		} else {
			b.WriteString("\n")
		}
	}
	b.WriteString(barPad(styleBar.Render(" "), w))
	return b.String()
}

// openSelected shows the tree's selected entry in the diff pane.
func (a *app) openSelected() {
	a.file, a.openedRel, a.note = nil, "", ""
	e := a.dir.selected()
	if e == nil {
		a.note = "nothing to compare"
		if a.dir.status != "" {
			a.note = a.dir.status
		}
		return
	}
	lp := filepath.Join(a.dir.leftRoot, e.rel)
	rp := filepath.Join(a.dir.rightRoot, e.rel)
	if e.status == stDeleted {
		a.note = "file deleted (u in the tree restores it)"
		a.openedRel = e.rel
		return
	}
	if isBinary(lp) || isBinary(rp) {
		a.note = "binary file — not shown"
		a.openedRel = e.rel
		return
	}
	m, err := newModel(lp, rp)
	if err != nil {
		a.note = "error: " + err.Error()
		return
	}
	m.roLeft, m.roRight = a.dir.roLeft, a.dir.roRight
	// name each side from the first path component the roots differ in,
	// so equal basenames (…/a/plugin vs …/b/plugin) stay distinguishable
	lt, rt := distinctTails(displayPath(a.dir.leftRoot), displayPath(a.dir.rightRoot))
	m.leftName = filepath.Join(lt, e.rel)
	m.rightName = filepath.Join(rt, e.rel)
	if a.dir.leftLabel != "" {
		m.leftName = a.dir.leftLabel + ":" + e.rel
		m.rightName = filepath.Join(displayPath(a.dir.rightRoot), e.rel)
	}
	if a.dir.roRight {
		m.rightName = a.dir.rightLabel + ":" + e.rel
	}
	if sidecar.Path != "" { // match notes by the tree path, not the cwd-relative one
		m.noteKey = e.rel
		m.loadFileNotes()
		m.recompute()
	}
	a.file, a.openedRel = m, e.rel
	a.layout()
	m.scrollToCur()
}

// Mode selects what Run compares.
type Mode int

const (
	ModeFiles Mode = iota // two files or two directories
	ModeGit               // working tree (or a second ref) against a git ref
	ModeMerge             // 3-way merge for git mergetool
)

// Options is what the command line resolved to.
type Options struct {
	Theme   string
	Mode    Mode
	Exclude []string // extra ignore patterns for this run
	Notes   string   // notes sidecar; empty = auto-detect
	Args    []string // positional arguments of the mode
}

// ErrUsage reports wrong positional arguments; the caller prints usage.
var ErrUsage = errors.New("usage")

// LoadConfig reads the user's config and returns the configured theme, so
// the command line can offer it as the flag default.
func LoadConfig() string {
	loadConfig()
	return cfg.Theme
}

// ThemeNames lists the available themes for usage text.
func ThemeNames() string { return themeNames() }

// KeysJSON renders the active key bindings as a config.json snippet.
func KeysJSON() string { return keysJSON() }

// Run starts the interactive program for o and returns the process exit
// code: 1 when a merge still has conflicts (git mergetool's trustExitCode).
func Run(o Options) (int, error) {
	notesPath := o.Notes
	if notesPath == "" {
		notesPath = defaultNotesPath()
	}
	if notesPath != "" {
		if err := sidecar.Load(notesPath); err != nil {
			return 1, err
		}
		defer notesSummary()
	}
	for _, w := range keyWarnings {
		fmt.Fprintln(os.Stderr, "difftool: config:", w)
	}
	t, ok := themes[o.Theme]
	if !ok {
		return 1, fmt.Errorf("unknown theme %q (themes: %s)", o.Theme, themeNames())
	}
	cfg.Theme = o.Theme
	initStyles(t)
	extraIgnores = append(extraIgnores, o.Exclude...)
	args := o.Args
	switch o.Mode {
	case ModeMerge:
		if len(args) != 4 {
			return 2, ErrUsage
		}
		m, err := newMergeModel(args[0], args[1], args[2], args[3])
		if err != nil {
			return 1, err
		}
		if err := runProgram(&app{file: m}); err != nil {
			return 1, err
		}
		if m.conflicts() > 0 {
			return 1, nil
		}
		return 0, nil
	case ModeGit:
		if len(args) > 2 {
			return 2, ErrUsage
		}
		ref, pathspec := "HEAD", ""
		switch len(args) {
		case 1:
			// a single arg is a pathspec when it exists on disk, else a ref;
			// use the two-arg form to disambiguate
			if _, err := os.Stat(args[0]); err == nil {
				pathspec = args[0]
			} else {
				ref = args[0]
			}
		case 2:
			ref, pathspec = args[0], args[1]
		}
		cwd, err := os.Getwd()
		if err != nil {
			return 1, err
		}
		if err := runGitMode(ref, cwd, pathspec); err != nil {
			return 1, err
		}
		return 0, nil
	}
	if len(args) != 2 {
		return 2, ErrUsage
	}
	lp, rp := args[0], args[1]
	li, err := os.Stat(lp)
	if err != nil {
		return 1, err
	}
	ri, err := os.Stat(rp)
	if err != nil {
		return 1, err
	}
	if li.IsDir() != ri.IsDir() {
		return 1, fmt.Errorf("cannot compare a directory with a file")
	}
	a := &app{}
	if li.IsDir() {
		if a.dir, err = newDirModel(lp, rp); err != nil {
			return 1, err
		}
	} else {
		if a.file, err = newModel(lp, rp); err != nil {
			return 1, err
		}
	}
	if err := runProgram(a); err != nil {
		return 1, err
	}
	return 0, nil
}

// defaultNotesPath finds the sidecar an agent left for this review: in the
// repository root when inside one, else in the working directory.
func defaultNotesPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	if root, err := gitCmd(dir, "rev-parse", "--show-toplevel"); err == nil {
		dir = root
	}
	p := filepath.Join(dir, ".difftool-notes.json")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// notesSummary prints one line on exit so a wrapper (difftool-review) can
// wait for it and an agent sees what the review left behind.
func notesSummary() {
	yours, resolved := 0, 0
	for _, n := range sidecar.Items {
		if n.Author == "human" && !n.Resolved {
			yours++
		}
		if n.Resolved {
			resolved++
		}
	}
	fmt.Fprintf(os.Stderr, "difftool: review closed · %d notes from you · %d resolved · %s\n", yours, resolved, sidecar.Path)
}

func runProgram(a *app) error {
	_, err := tea.NewProgram(a, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func themeNames() string { return strings.Join(sortedThemes(), ", ") }
