package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// closeFileMsg is emitted by an embedded file diff view when the user
// closes it to return to the directory list.
type closeFileMsg struct{}

// switchFileMsg asks the app to open the next/previous file of the dir list.
type switchFileMsg struct{ delta int }

type app struct {
	dir  *dirModel
	file *model
	w, h int

	menuOpen bool
	menuSel  int
	helpOpen bool
}

func (a *app) Init() tea.Cmd { return nil }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		if a.dir != nil {
			a.dir.w, a.dir.h = a.w, a.h
			a.dir.ensureVisible()
		}
		if a.file != nil {
			a.file.w, a.file.h = a.w, a.h
			a.file.clampScroll()
		}
		return a, nil
	case closeFileMsg:
		a.file = nil
		a.dir.refreshSelected()
		return a, nil
	case switchFileMsg:
		if a.dir != nil {
			a.file = nil
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
		}
	}
	if a.file != nil {
		_, cmd := a.file.Update(msg)
		return a, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "enter" {
		a.openSelected()
		return a, nil
	}
	return a, a.dir.update(msg)
}

// inputActive reports whether a text input (search or filter) is capturing
// keys, so app-level shortcuts must stay out of the way.
func (a *app) inputActive() bool {
	return (a.file != nil && a.file.searchInput) || (a.dir != nil && a.file == nil && a.dir.filterInput)
}

func (a *app) View() string {
	if a.helpOpen {
		return a.helpView()
	}
	if a.menuOpen {
		return a.settingsView()
	}
	if a.file != nil {
		return a.file.View()
	}
	return a.dir.view()
}

func (a *app) openSelected() {
	e := a.dir.selected()
	if e == nil {
		return
	}
	lp := filepath.Join(a.dir.leftRoot, e.rel)
	rp := filepath.Join(a.dir.rightRoot, e.rel)
	if isBinary(lp) || isBinary(rp) {
		a.dir.status = "binary file — not opening diff"
		return
	}
	m, err := newModel(lp, rp)
	if err != nil {
		a.dir.status = "error: " + err.Error()
		return
	}
	m.embedded = true
	m.roLeft = a.dir.roLeft
	if a.dir.leftLabel != "" {
		m.leftName = a.dir.leftLabel + ":" + e.rel
	}
	m.w, m.h = a.w, a.h
	m.scrollToCur()
	a.file = m
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
