package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
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
	IgnorePatterns []string `json:"ignore_patterns"`
	UseIgnores     bool     `json:"use_ignores"`
}

// extraIgnores holds patterns from the -x flag; never persisted.
var extraIgnores []string

var cfg = defaultConfig()

func defaultConfig() config {
	return config{
		Theme: "rose-pine", Intraline: true, TabWidth: 4,
		UseIgnores: true, Syntax: true,
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
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
			names := make([]string, 0, len(themes))
			for n := range themes {
				names = append(names, n)
			}
			sort.Strings(names)
			i := 0
			for j, n := range names {
				if n == cfg.Theme {
					i = j
				}
			}
			cfg.Theme = names[(i+d+len(names))%len(names)]
			a.applySettings()
		}},
		{"syntax highlighting", func() string { return onOff(cfg.Syntax) }, func(int) {
			cfg.Syntax = !cfg.Syntax
			a.applySettings()
		}},
		{"intraline highlight", func() string { return onOff(cfg.Intraline) }, func(int) {
			cfg.Intraline = !cfg.Intraline
			a.applySettings()
		}},
		{"ignore whitespace", func() string { return onOff(cfg.IgnoreWs) }, func(int) {
			cfg.IgnoreWs = !cfg.IgnoreWs
			a.applySettings()
		}},
		{"tab width", func() string { return fmt.Sprint(cfg.TabWidth) }, func(d int) {
			widths := []int{2, 4, 8}
			i := 0
			for j, w := range widths {
				if w == cfg.TabWidth {
					i = j
				}
			}
			cfg.TabWidth = widths[(i+d+len(widths))%len(widths)]
			a.applySettings()
		}},
		{"show identical files (dirs)", func() string { return onOff(cfg.ShowIdentical) }, func(int) {
			cfg.ShowIdentical = !cfg.ShowIdentical
			a.applySettings()
		}},
		{"ignore patterns (dirs)", ignoreSummary, func(int) {
			cfg.UseIgnores = !cfg.UseIgnores
			a.applySettings()
			a.rescanDir()
		}},
	}
}

// applySettings makes setting changes take effect immediately.
func (a *app) applySettings() {
	initStyles(themes[cfg.Theme])
	if a.file != nil {
		a.file.recompute()
	}
	if a.dir != nil {
		a.dir.showAll = cfg.ShowIdentical
		a.dir.rebuildList()
	}
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
	case "h", "left":
		items[a.menuSel].change(-1)
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
		name := it.name + strings.Repeat(" ", max(1, 32-len(it.name)))
		if i == a.menuSel {
			b.WriteString(styleMark.Render("▌") +
				styleSelected.Render(padCell(" "+name+" ", 0)) +
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
