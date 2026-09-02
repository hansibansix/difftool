package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	actions []string // rendered from the current key bindings; nil = literal key
	key     string
	desc    string
}

type helpSection struct {
	title   string
	km      keymap
	entries []helpEntry
}

func acts(a ...string) []string { return a }

// helpSections lists every action per context; the keys shown come from the
// active bindings so a customized layout documents itself.
func helpSections() []helpSection {
	return []helpSection{
		{"file view", keys.file, []helpEntry{
			{acts("next", "prev"), "", "next / previous change or applied hunk"},
			{acts("apply-right"), "", "apply current chunk left → right"},
			{acts("apply-left"), "", "apply current chunk right → left"},
			{acts("apply-all"), "", "apply ALL pending chunks (then the ▶ or ◀ apply key)"},
			{acts("visual"), "", "select lines in the chunk (down/up), then apply only those"},
			{acts("reset", "reset-all"), "", "reset applied hunk under cursor / all"},
			{acts("undo"), "", "undo last apply, reset or apply-all"},
			{acts("save"), "", "save modified file(s)"},
			{acts("edit-right", "edit-left"), "", "edit right / left file in $EDITOR at the current hunk"},
			{acts("patch"), "", "export pending hunks as a patch (clipboard, else ./difftool.patch); in visual mode only the current hunk"},
			{acts("search", "next", "search-prev"), "", "search, next / previous match (esc clears)"},
			{acts("intraline"), "", "toggle intraline highlight"},
			{acts("wrap"), "", "toggle line wrap"},
			{acts("unified"), "", "toggle unified (one-column) view"},
			{acts("fold"), "", "fold unchanged lines (click a fold to expand it)"},
			{acts("next-file", "prev-file"), "", "next / previous file (directory mode)"},
			{acts("merge-local", "merge-base", "merge-remote"), "", "merge mode: show LOCAL / BASE / REMOTE on the left"},
			{acts("down", "up", "top", "bottom"), "", "scroll; half pages with the half-down / half-up keys"},
			{acts("half-down", "half-up", "scroll-left", "scroll-right"), "", "half page down / up, scroll left / right"},
			{acts("tree"), "", "back to the tree pane (directory mode)"},
			{acts("quit"), "", "close (asks once on unsaved changes)"},
		}},
		{"directory view", keys.dir, []helpEntry{
			{acts("open"), "", "focus the diff pane"},
			{acts("down", "up"), "", "select file; the diff pane follows"},
			{acts("copy-right", "copy-left"), "", "copy file to the other side; on a one-sided file the other direction deletes it (asks y/n)"},
			{acts("sync-all"), "", "sync ALL listed files one way (then the ▶ or ◀ copy key, confirm y)"},
			{acts("undo"), "", "undo last copy"},
			{acts("filter"), "", "filter list (esc clears)"},
			{acts("identical"), "", "toggle identical files"},
			{acts("top", "bottom", "half-down", "half-up"), "", "move selection"},
			{acts("quit"), "", "quit"},
		}},
		{"everywhere", keys.global, []helpEntry{
			{acts("settings"), "", "settings (theme, whitespace, blank-line / regex ignore, …); enter on a list item edits it"},
			{acts("help"), "", "this help (j/k scroll)"},
			{acts("tree-toggle"), "", "show / hide the tree pane"},
			{acts("ignore-add"), "", "add an ignore pattern (prefilled with the selected file name)"},
			{nil, "mouse", "click a file or hunk to select it, wheel scrolls the pane under the pointer"},
			{nil, "", "key bindings are customizable: difftool -keys prints the config snippet"},
		}},
	}
}

func (a *app) helpView() string {
	if a.w == 0 || a.h == 0 {
		return ""
	}
	keySt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.accent)).Bold(true)
	secs := helpSections()
	keyW := 12
	for _, sec := range secs {
		for _, e := range sec.entries {
			if e.actions != nil {
				e.key = keyLabel(sec.km, e.actions...)
			}
			keyW = max(keyW, lipgloss.Width(e.key)+2)
		}
	}
	var body []string
	for _, sec := range secs {
		body = append(body, "", "  "+styleGroup.Render(sec.title))
		for _, e := range sec.entries {
			if e.actions != nil {
				e.key = keyLabel(sec.km, e.actions...)
			}
			body = append(body, "  "+keySt.Render(padCell(e.key, keyW))+e.desc)
		}
	}
	bodyH := max(1, a.h-2)
	a.helpTop = clamp(a.helpTop, 0, max(0, len(body)-bodyH))
	more := len(body) > a.helpTop+bodyH
	body = body[a.helpTop:min(len(body), a.helpTop+bodyH)]
	if more {
		body[len(body)-1] = styleGutter.Render("  ⋯ j/k scroll")
	}
	var b strings.Builder
	b.WriteString(barPad(styleBar.Render(" ")+styleHeaderText.Render("help"), a.w) + "\n")
	for _, l := range body {
		b.WriteString(l + "\n")
	}
	for i := len(body); i < bodyH; i++ {
		b.WriteString("\n")
	}
	b.WriteString(footerBar(a.w, "", "", [][2]string{{"j/k", "scroll"}, {"any other key", "close"}}))
	return b.String()
}
