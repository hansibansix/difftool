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
	open   func() // optional: enter opens a sub-view instead of changing
}

func (a *app) menuItems() []menuItem {
	items := []menuItem{
		{"theme", func() string { return cfg.Theme }, func(d int) {
			cfg.Theme = cycle(sortedThemes(), cfg.Theme, d)
		}, nil},
		{"syntax highlighting", func() string { return onOff(cfg.Syntax) }, func(int) {
			cfg.Syntax = !cfg.Syntax
		}, nil},
		{"intraline highlight", func() string { return onOff(cfg.Intraline) }, func(int) {
			cfg.Intraline = !cfg.Intraline
		}, nil},
		{"line wrap", func() string { return onOff(cfg.Wrap) }, func(int) {
			cfg.Wrap = !cfg.Wrap
		}, nil},
		{"ignore whitespace", func() string { return onOff(cfg.IgnoreWs) }, func(int) {
			cfg.IgnoreWs = !cfg.IgnoreWs
		}, nil},
		{"tab width", func() string { return fmt.Sprint(cfg.TabWidth) }, func(d int) {
			cfg.TabWidth = cycle([]int{2, 4, 8}, cfg.TabWidth, d)
		}, nil},
		{"show identical files (dirs)", func() string { return onOff(cfg.ShowIdentical) }, func(int) {
			cfg.ShowIdentical = !cfg.ShowIdentical
		}, nil},
		{"tree pane (dirs)", func() string { return onOff(cfg.ShowTree) }, func(int) {
			a.toggleTree()
		}, nil},
		{"ignore patterns (dirs) · enter edits", ignoreSummary, func(int) {
			cfg.UseIgnores = !cfg.UseIgnores
			a.rescanDir()
		}, nil},
	}
	items[len(items)-1].open = func() { a.ignEdit = true }
	return items
}

// --- ignore pattern editor (sub-view of the settings menu) ---

func (a *app) addIgnore(p string) {
	p = strings.TrimSpace(p)
	if p == "" {
		return
	}
	for _, q := range cfg.IgnorePatterns {
		if q == p {
			return
		}
	}
	cfg.IgnorePatterns = append(cfg.IgnorePatterns, p)
	a.ignSel = len(cfg.IgnorePatterns) - 1
	a.ignoreChanged()
}

func (a *app) removeIgnore(i int) {
	if i < 0 || i >= len(cfg.IgnorePatterns) {
		return
	}
	cfg.IgnorePatterns = append(cfg.IgnorePatterns[:i], cfg.IgnorePatterns[i+1:]...)
	a.ignSel = clamp(a.ignSel, 0, max(0, len(cfg.IgnorePatterns)-1))
	a.ignoreChanged()
}

func (a *app) ignoreChanged() {
	a.applySettings()
	a.rescanDir()
	saveConfig()
}

func (a *app) updateIgnoreEditor(k tea.KeyMsg) {
	if a.ignInput {
		switch k.String() {
		case "enter":
			a.addIgnore(a.ignText)
			a.ignInput = false
		case "esc", "ctrl+c":
			a.ignInput = false
		default:
			a.ignText = editText(a.ignText, k)
		}
		return
	}
	switch k.String() {
	case "q", "esc":
		a.ignEdit = false
	case "j", "down":
		a.ignSel = min(a.ignSel+1, max(0, len(cfg.IgnorePatterns)-1))
	case "k", "up":
		a.ignSel = max(a.ignSel-1, 0)
	case "a", "n", "+", "i", "enter":
		a.ignInput, a.ignText = true, ""
	case "d", "x", "backspace", "delete":
		a.removeIgnore(a.ignSel)
	}
}

func (a *app) ignoreEditorView() string {
	var b strings.Builder
	b.WriteString(barPad(styleBar.Render(" ")+styleHeaderText.Render("ignore patterns"), a.w) + "\n")
	bodyH := max(1, a.h-2)
	body := []string{styleGutter.Render("  globs; without / they match the basename at any depth, with / the relative path")}
	if a.ignInput {
		body = append(body, "  "+styleStatus.Render("new pattern: ")+a.ignText+"▏")
	}
	body = append(body, "")
	var list []string
	for i, p := range cfg.IgnorePatterns {
		if i == a.ignSel && !a.ignInput {
			list = append(list, styleMark.Render("▌")+styleSelected.Render(padCell(" "+p, 40)))
		} else {
			list = append(list, "  "+p)
		}
	}
	for _, p := range extraIgnores {
		list = append(list, "  "+styleGutter.Render(p+"  (from -x, this run only)"))
	}
	if len(cfg.IgnorePatterns) == 0 {
		list = append(list, styleGutter.Render("  (none)"))
	}
	// window the list around the selection so it fits the remaining height
	room := max(1, bodyH-len(body))
	start := clamp(a.ignSel-room/2, 0, max(0, len(list)-room))
	list = list[start:min(len(list), start+room)]
	body = append(body, list...)
	for _, l := range body {
		b.WriteString(l + "\n")
	}
	for i := len(body); i < bodyH; i++ {
		b.WriteString("\n")
	}
	hints := [][2]string{{"a", "add"}, {"d", "delete"}, {"j/k", "move"}, {"q", "back to settings"}}
	if a.ignInput {
		hints = [][2]string{{"enter", "add pattern"}, {"esc", "cancel"}}
	}
	b.WriteString(footerBar(a.w, "", "", hints))
	return b.String()
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
	if a.ignEdit {
		a.updateIgnoreEditor(k)
		return a, nil
	}
	items := a.menuItems()
	switch k.String() {
	case "enter":
		if it := items[a.menuSel]; it.open != nil {
			it.open()
		} else {
			it.change(1)
			a.applySettings()
		}
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
	case "l", "right", " ":
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
	if a.ignEdit {
		return a.ignoreEditorView()
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
		{"j/k", "move"}, {"h·l", "change"}, {"enter", "change / edit"}, {"q", "close & save"},
	}))
	return b.String()
}
