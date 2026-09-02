package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
}

func (a *app) Init() tea.Cmd { return nil }

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

func (a *app) fileDirty() bool {
	return a.file != nil && (a.file.dirtyL || a.file.dirtyR)
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.layout()
		if a.dir != nil && a.file == nil && a.note == "" {
			a.openSelected()
		}
		return a, nil
	case closeFileMsg:
		a.focusDiff = false
		a.dir.refreshSelected()
		return a, nil
	case switchFileMsg:
		if a.dir != nil {
			a.dir.refreshSelected()
			a.dir.move(msg.delta)
			a.openSelected()
		}
		return a, nil
	}
	if a.helpOpen {
		if _, ok := msg.(tea.KeyMsg); ok {
			a.helpOpen = false
		}
		return a, nil
	}
	if a.menuOpen {
		return a.updateMenu(msg)
	}
	if k, ok := msg.(tea.KeyMsg); ok && !a.inputActive() {
		switch k.String() {
		case ",":
			a.menuOpen = true
			return a, nil
		case "?":
			a.helpOpen = true
			return a, nil
		case "t":
			if a.dir != nil {
				a.toggleTree()
				return a, nil
			}
		}
	}
	if a.dir == nil {
		_, cmd := a.file.Update(msg)
		return a, cmd
	}
	if a.focusDiff && a.file != nil {
		if k, ok := msg.(tea.KeyMsg); ok && k.String() == "tab" && !a.file.searchInput {
			a.focusDiff = false
			a.dir.refreshSelected()
			return a, nil
		}
		wasDirty := a.fileDirty()
		_, cmd := a.file.Update(msg)
		if wasDirty && !a.fileDirty() {
			a.dir.refreshSelected() // saved: update the tree status
		}
		return a, cmd
	}
	// tree pane focused
	if k, ok := msg.(tea.KeyMsg); ok && !a.dir.filterInput {
		switch k.String() {
		case "enter", "tab":
			if a.file == nil {
				a.openSelected()
			}
			if a.file != nil {
				a.focusDiff = true
			} else if a.note != "" {
				a.dir.status = a.note
			}
			return a, nil
		case "h", "l", "left", "right", "<", ">":
			if a.fileDirty() {
				a.dir.status = "unsaved changes in " + filepath.Base(a.openedRel) + " — save or undo first"
				return a, nil
			}
			cmd := a.dir.update(msg)
			a.openSelected() // the copy changed the file on disk: show the new state
			return a, cmd
		}
	}
	prev := a.dir.selected()
	prevRel := ""
	if prev != nil {
		prevRel = prev.rel
	}
	cmd := a.dir.update(msg)
	if cur := a.dir.selected(); cur != nil && cur.rel != a.openedRel {
		if a.fileDirty() {
			a.dir.status = "unsaved changes in " + filepath.Base(a.openedRel) + " — save or undo first"
			a.dir.selectRel(prevRel)
		} else {
			a.openSelected()
		}
	}
	return a, cmd
}

// inputActive reports whether a text input (search or filter) is capturing
// keys, so app-level shortcuts must stay out of the way.
func (a *app) inputActive() bool {
	if a.dir == nil {
		return a.file.searchInput
	}
	if a.focusDiff && a.file != nil {
		return a.file.searchInput
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
		a.file.focused = true
		return a.file.View()
	}
	a.dir.focused = !a.focusDiff
	if a.file != nil {
		a.file.focused = a.focusDiff
	}
	if !a.split() {
		if a.focusDiff && a.file != nil {
			return a.file.View()
		}
		return a.dir.view()
	}
	right := a.placeholder()
	if a.file != nil {
		right = a.file.View()
	}
	sep := strings.TrimSuffix(strings.Repeat(styleSep.Render("│")+"\n", a.h), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, a.dir.view(), sep, right)
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
	m.embedded = true
	m.roLeft = a.dir.roLeft
	m.leftName = filepath.Join(filepath.Base(a.dir.leftRoot), e.rel)
	m.rightName = filepath.Join(filepath.Base(a.dir.rightRoot), e.rel)
	if a.dir.leftLabel != "" {
		m.leftName = a.dir.leftLabel + ":" + e.rel
	}
	a.file, a.openedRel = m, e.rel
	a.layout()
	m.scrollToCur()
}

func main() {
	loadConfig()
	themeName := flag.String("theme", envOr("DIFFTOOL_THEME", cfg.Theme), "color theme")
	gitMode := flag.Bool("git", false, "compare working tree against a git ref (default HEAD)")
	exclude := flag.String("x", "", "additional ignore patterns, comma-separated globs")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: difftool [-theme name] <left> <right>  (two files or two directories)")
		fmt.Fprintln(os.Stderr, "       difftool [-theme name] -git [ref] [path]  (working tree vs. git ref)")
		fmt.Fprintf(os.Stderr, "themes: %s\n", themeNames())
	}
	flag.Parse()
	t, ok := themes[*themeName]
	if !ok {
		fatal(fmt.Errorf("unknown theme %q (themes: %s)", *themeName, themeNames()))
	}
	cfg.Theme = *themeName
	initStyles(t)
	for _, p := range strings.Split(*exclude, ",") {
		if p = strings.TrimSpace(p); p != "" {
			extraIgnores = append(extraIgnores, p)
		}
	}
	if *gitMode {
		if flag.NArg() > 2 {
			flag.Usage()
			os.Exit(2)
		}
		ref, pathspec := "HEAD", ""
		switch flag.NArg() {
		case 1:
			// a single arg is a pathspec when it exists on disk, else a ref;
			// use the two-arg form to disambiguate
			if _, err := os.Stat(flag.Arg(0)); err == nil {
				pathspec = flag.Arg(0)
			} else {
				ref = flag.Arg(0)
			}
		case 2:
			ref, pathspec = flag.Arg(0), flag.Arg(1)
		}
		cwd, err := os.Getwd()
		if err != nil {
			fatal(err)
		}
		runGitMode(ref, cwd, pathspec)
		return
	}
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	lp, rp := flag.Arg(0), flag.Arg(1)
	li, err := os.Stat(lp)
	if err != nil {
		fatal(err)
	}
	ri, err := os.Stat(rp)
	if err != nil {
		fatal(err)
	}
	if li.IsDir() != ri.IsDir() {
		fatal(fmt.Errorf("cannot compare a directory with a file"))
	}
	a := &app{}
	if li.IsDir() {
		if a.dir, err = newDirModel(lp, rp); err != nil {
			fatal(err)
		}
	} else {
		if a.file, err = newModel(lp, rp); err != nil {
			fatal(err)
		}
	}
	if err := runProgram(a); err != nil {
		fatal(err)
	}
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

func themeNames() string {
	names := make([]string, 0, len(themes))
	for n := range themes {
		names = append(names, n)
	}
	sort.Strings(names)
	s := ""
	for i, n := range names {
		if i > 0 {
			s += ", "
		}
		s += n
	}
	return s
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "difftool:", err)
	os.Exit(1)
}
