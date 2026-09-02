package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct{ key, desc string }

var helpSections = []struct {
	title   string
	entries []helpEntry
}{
	{"file view", []helpEntry{
		{"n / p", "next / previous change or applied hunk"},
		{"l → >", "apply current chunk left → right"},
		{"h ← <", "apply current chunk right → left"},
		{"a", "apply ALL pending chunks (then l ▶ or h ◀)"},
		{"v", "select lines in the chunk (j/k), then l/h applies only those"},
		{"x / X", "reset applied hunk under cursor / all"},
		{"u", "undo last apply, reset or apply-all"},
		{"s", "save modified file(s)"},
		{"/ · n/N", "search, next/previous match (esc clears)"},
		{"i", "toggle intraline highlight"},
		{"w", "toggle line wrap"},
		{"J / K", "next / previous file (directory mode)"},
		{"j k g G ^d ^u", "scroll, H / L horizontally"},
		{"q · esc", "close (asks once on unsaved changes)"},
	}},
	{"directory view", []helpEntry{
		{"enter · tab", "focus the diff pane (tab returns)"},
		{"j / k", "select file; the diff pane follows"},
		{"t", "show / hide the tree pane"},
		{"l / h", "copy file to the other side (immediately)"},
		{"u", "undo last copy"},
		{"/", "filter list (esc clears)"},
		{"a", "toggle identical files"},
		{"g G ^d ^u", "move selection"},
		{"q · esc", "quit"},
	}},
	{"everywhere", []helpEntry{
		{",", "settings (theme, whitespace, ignores, …)"},
		{"?", "this help"},
		{"mouse", "click a file or hunk to select it, wheel scrolls the pane under the pointer"},
	}},
}

func (a *app) helpView() string {
	if a.w == 0 || a.h == 0 {
		return ""
	}
	keySt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.accent)).Bold(true)
	var body []string
	for _, sec := range helpSections {
		body = append(body, "", "  "+styleGroup.Render(sec.title))
		for _, e := range sec.entries {
			body = append(body, "  "+keySt.Render(padCell(e.key, 15))+e.desc)
		}
	}
	bodyH := max(1, a.h-2)
	if len(body) > bodyH {
		body = append(body[:bodyH-1], styleGutter.Render("  … (enlarge the window for the full list)"))
	}
	var b strings.Builder
	b.WriteString(barPad(styleBar.Render(" ")+styleHeaderText.Render("help"), a.w) + "\n")
	for _, l := range body {
		b.WriteString(l + "\n")
	}
	for i := len(body); i < bodyH; i++ {
		b.WriteString("\n")
	}
	b.WriteString(footerBar(a.w, "", "", [][2]string{{"any key", "close"}}))
	return b.String()
}
