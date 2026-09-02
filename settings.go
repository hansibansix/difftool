package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type config struct {
	Theme          string   `json:"theme"`
	Intraline      bool     `json:"intraline"`
	IgnoreWs       bool     `json:"ignore_whitespace"`
	TabWidth       int      `json:"tab_width"`
	ShowIdentical  bool     `json:"show_identical"`
	Syntax         bool     `json:"syntax"`
	ShowTree       bool     `json:"show_tree"`
	Wrap           bool     `json:"wrap"`
	IgnorePatterns []string `json:"ignore_patterns"`
	UseIgnores     bool     `json:"use_ignores"`
}

// extraIgnores holds patterns from the -x flag; never persisted.
var extraIgnores []string

var cfg = defaultConfig()

func defaultConfig() config {
	return config{
		Theme: "rose-pine", Intraline: true, TabWidth: 4,
		UseIgnores: true, Syntax: true, ShowTree: true,
		IgnorePatterns: []string{
			"node_modules", "vendor", // dependency trees
			".svn", ".hg", // VCS metadata (.git is always skipped)
			".idea", ".vscode", // IDE state
			"__pycache__", "*.pyc",
			"*.min.js", "*.min.js.map", "*.min.css", // generated assets
			"*.swp", ".DS_Store", ".cache",
		},
	}
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "difftool", "config.json")
}

func loadConfig() { loadConfigFrom(configPath()) }

func loadConfigFrom(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &cfg)
	if _, ok := themes[cfg.Theme]; !ok {
		cfg.Theme = defaultConfig().Theme
	}
	if cfg.TabWidth != 2 && cfg.TabWidth != 4 && cfg.TabWidth != 8 {
		cfg.TabWidth = defaultConfig().TabWidth
	}
}

func saveConfig() error { return saveConfigTo(configPath()) }

func saveConfigTo(path string) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileMkdir(path, append(data, '\n'), 0o644)
}

// ignored reports whether a slash-relative path matches an ignore pattern.
// Patterns containing a slash match against the relative path, others
// against the basename (glob syntax, * does not cross /).
func ignored(rel string) bool {
	if !cfg.UseIgnores {
		return false
	}
	rel = filepath.ToSlash(rel)
	base := path.Base(rel)
	match := func(pats []string) bool {
		for _, p := range pats {
			if strings.ContainsRune(p, '/') {
				if ok, _ := path.Match(p, rel); ok {
					return true
				}
			} else if ok, _ := path.Match(p, base); ok {
				return true
			}
		}
		return false
	}
	return match(cfg.IgnorePatterns) || match(extraIgnores)
}

func ignoreSummary() string {
	if !cfg.UseIgnores {
		return "off"
	}
	pats := append(append([]string(nil), cfg.IgnorePatterns...), extraIgnores...)
	if len(pats) == 0 {
		return "on (none)"
	}
	s := strings.Join(pats, ",")
	if len(s) > 20 {
		s = s[:19] + "…"
	}
	return s
}

// normalizeWs collapses whitespace runs and trims, for ignore-whitespace diffs.
func normalizeWs(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.Join(strings.Fields(l), " ")
	}
	return out
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

type menuItem struct {
	name   string
	value  func() string
	change func(delta int)
}

func (a *app) menuItems() []menuItem {
	return []menuItem{
		{"theme", func() string { return cfg.Theme }, func(d int) {
			cfg.Theme = cycle(sortedThemes(), cfg.Theme, d)
		}},
		{"syntax highlighting", func() string { return onOff(cfg.Syntax) }, func(int) {
			cfg.Syntax = !cfg.Syntax
		}},
		{"intraline highlight", func() string { return onOff(cfg.Intraline) }, func(int) {
			cfg.Intraline = !cfg.Intraline
		}},
		{"line wrap", func() string { return onOff(cfg.Wrap) }, func(int) {
			cfg.Wrap = !cfg.Wrap
		}},
		{"ignore whitespace", func() string { return onOff(cfg.IgnoreWs) }, func(int) {
			cfg.IgnoreWs = !cfg.IgnoreWs
		}},
		{"tab width", func() string { return fmt.Sprint(cfg.TabWidth) }, func(d int) {
			cfg.TabWidth = cycle([]int{2, 4, 8}, cfg.TabWidth, d)
		}},
		{"show identical files (dirs)", func() string { return onOff(cfg.ShowIdentical) }, func(int) {
			cfg.ShowIdentical = !cfg.ShowIdentical
		}},
		{"tree pane (dirs)", func() string { return onOff(cfg.ShowTree) }, func(int) {
			a.toggleTree()
		}},
		{"ignore patterns (dirs)", ignoreSummary, func(int) {
			cfg.UseIgnores = !cfg.UseIgnores
			a.rescanDir()
		}},
	}
}

// cycle returns the element d steps after cur in xs, wrapping around.
func cycle[T comparable](xs []T, cur T, d int) T {
	i := 0
	for j, x := range xs {
		if x == cur {
			i = j
		}
	}
	return xs[((i+d)%len(xs)+len(xs))%len(xs)]
}

// applySettings makes setting changes take effect immediately.
func (a *app) applySettings() {
	initStyles(themes[cfg.Theme])
	if a.file != nil {
		a.file.recompute()
	}
	if a.dir != nil {
		a.dir.rebuildList()
	}
}

// toggleTree shows/hides the tree pane in dir mode; hiding moves focus to
// the diff so the tree does not stay full-screen.
func (a *app) toggleTree() {
	cfg.ShowTree = !cfg.ShowTree
	if !cfg.ShowTree && a.file != nil {
		a.focusDiff = true
	}
	a.layout()
	if a.dir != nil {
		a.dir.status = "tree pane " + onOff(cfg.ShowTree)
	}
	saveConfig()
}

// rescanDir re-walks the compared directories, e.g. after the ignore toggle
// changed; in git mode the entry list is fixed, rebuildList filters it.
func (a *app) rescanDir() {
	if a.dir == nil || a.dir.roLeft {
		return
	}
	a.dir.status = ""
	if err := a.dir.scan(); err != nil {
		a.dir.status = "error: " + err.Error()
	}
}

func (a *app) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	items := a.menuItems()
	switch k.String() {
	case "q", "esc", ",":
		a.menuOpen = false
		saveConfig()
	case "ctrl+c":
		saveConfig()
		return a, tea.Quit
	case "j", "down":
		a.menuSel = min(a.menuSel+1, len(items)-1)
	case "k", "up":
		a.menuSel = max(a.menuSel-1, 0)
	case "l", "right", "enter", " ":
		items[a.menuSel].change(1)
		a.applySettings()
	case "h", "left":
		items[a.menuSel].change(-1)
		a.applySettings()
	}
	return a, nil
}

func (a *app) settingsView() string {
	if a.w == 0 || a.h == 0 {
		return ""
	}
	valSt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.accent))
	valSelSt := valSt.Background(lipgloss.Color(th.selBg))
	var b strings.Builder
	b.WriteString(barPad(styleBar.Render(" ")+styleHeaderText.Render("settings"), a.w) + "\n")
	items := a.menuItems()
	for i := 0; i < max(1, a.h-2); i++ {
		if i >= len(items) {
			b.WriteString("\n")
			continue
		}
		it := items[i]
		name := padCell(it.name, 32)
		if i == a.menuSel {
			b.WriteString(styleMark.Render("▌") +
				styleSelected.Render(" "+name+" ") +
				valSelSt.Render(padCell(it.value(), 12)) + "\n")
		} else {
			b.WriteString("  " + name + " " + valSt.Render(it.value()) + "\n")
		}
	}
	b.WriteString(footerBar(a.w, "", "", [][2]string{
		{"j/k", "move"}, {"h·l/enter", "change"}, {"q", "close & save"},
	}))
	return b.String()
}
