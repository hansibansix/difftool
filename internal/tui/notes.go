package tui

import (
	"fmt"
	"github.com/hansibansix/difftool/internal/notes"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Agent notes: a sidecar JSON file in hunk's --agent-context format
// (https://github.com/modem-dev/hunk) anchoring remarks to lines of the
// compared files. The file is polled so an agent can add notes while the
// diff is open, and notes typed in difftool are written back to it with
// author "human" for the agent to read.

// sidecar is the loaded notes file; an empty Path means the feature is off.
var sidecar notes.Store

// notesHidden hides every note box for the session (the gutter tint and
// the tree badges stay), for reading busy files plainly.
var notesHidden bool

// notePath is the filePath the notes of this file are matched against and
// written with: the tree's relative path in dir mode, else the patch name.
func (m *model) notePath() string {
	if m.noteKey != "" {
		return m.noteKey
	}
	return m.patchName()
}

// loadFileNotes picks this file's notes out of the store; called on open
// and after every reload since the pointers go stale.
func (m *model) loadFileNotes() {
	m.notes = nil
	if sidecar.Path != "" {
		m.notes = sidecar.For(m.notePath())
	}
	for _, n := range m.notes {
		m.resolveMatch(n)
	}
}

// resolveMatch re-anchors a note on the line its match text names, right
// side first; a range keeps its length. Line numbers stay as they are when
// the text is missing or ambiguous.
func (m *model) resolveMatch(n *notes.Note) {
	if n.Match == "" {
		return
	}
	start, _ := n.Line()
	if i := notes.FindLine(m.right, n.Match); i >= 0 {
		n.NewLine, n.OldLine = i+1, 0
	} else if i := notes.FindLine(m.left, n.Match); i >= 0 {
		n.NewLine, n.OldLine = 0, i+1
	} else {
		return
	}
	if n.EndLine > 0 && start > 0 {
		n.EndLine += max(n.NewLine, n.OldLine) - start
	}
}

// anchoredAt reports whether note k sits on row r.
func (m *model) anchoredAt(k int, r row) bool {
	n := m.notes[k]
	return (n.NewLine > 0 && r.r == n.NewLine-1) || (n.NewLine == 0 && n.OldLine > 0 && r.l == n.OldLine-1)
}

// hasNote reports whether a line pair (left l, right r) carries a note, so
// folding keeps it visible.
func (m *model) hasNote(l, r int) bool {
	for k := range m.notes {
		if m.anchoredAt(k, row{l: l, r: r}) {
			return true
		}
	}
	return false
}

// insertNoteRows places a note row above the row of each note's line (as
// hunk does: the note introduces the code it talks about); unanchored notes
// go to the top, notes past the end of the file to the bottom. Nav targets
// are re-pointed at their shifted rows.
func (m *model) insertNoteRows() {
	if len(m.notes) == 0 || notesHidden {
		return
	}
	out := make([]row, 0, len(m.rows)+len(m.notes))
	placed := make([]bool, len(m.notes))
	ci := 0
	for k, n := range m.notes {
		if n.NewLine == 0 && n.OldLine == 0 {
			out = append(out, row{l: -1, r: -1, ci: ci, note: k + 1})
			placed[k] = true
		}
	}
	newIdx := make([]int, len(m.rows))
	for i, r := range m.rows {
		ci = r.ci
		if r.fold == 0 {
			for k := range m.notes {
				if !placed[k] && m.anchoredAt(k, r) {
					out = append(out, row{l: -1, r: -1, ci: ci, note: k + 1})
					placed[k] = true
				}
			}
		}
		newIdx[i] = len(out)
		out = append(out, r)
	}
	for k := range m.notes {
		if !placed[k] {
			out = append(out, row{l: -1, r: -1, ci: ci, note: k + 1})
		}
	}
	m.rows = out
	for i := range m.nav {
		m.nav[i].row = newIdx[m.nav[i].row]
	}
}

// shiftNotes moves anchors below `from` on one side by delta lines, so
// notes follow an applied or reset hunk (called by model.replace).
func (m *model) shiftNotes(right bool, from, delta int) {
	for _, n := range m.notes {
		_, onRight := n.Line()
		if onRight == right && n.EndLine > from {
			n.EndLine += delta
		}
		if right && n.NewLine > from {
			n.NewLine += delta
		}
		if !right && n.OldLine > from {
			n.OldLine += delta
		}
	}
}

// noteAnchors snapshots the anchors for undo.
func (m *model) noteAnchors() [][3]int {
	out := make([][3]int, len(m.notes))
	for i, n := range m.notes {
		out[i] = [3]int{n.NewLine, n.OldLine, n.EndLine}
	}
	return out
}

func (m *model) restoreAnchors(a [][3]int) {
	if len(a) != len(m.notes) {
		return // the sidecar changed meanwhile; keep what it says
	}
	for i, n := range m.notes {
		n.NewLine, n.OldLine, n.EndLine = a[i][0], a[i][1], a[i][2]
	}
}

// noteAt returns the first open note whose range covers line idx (0-based)
// of one side, for tinting its line number.
func (m *model) noteAt(idx int, right bool) *notes.Note {
	for _, n := range m.notes {
		if !n.Resolved && n.Covers(idx+1, right) {
			return n
		}
	}
	return nil
}

// toggleResolved flips the note under the cursor between open and resolved
// and writes the sidecar.
func (m *model) toggleResolved() {
	if len(m.rows) == 0 || m.rows[m.curRow].note == 0 {
		m.status = "move onto a note (" + hint(keys.file, "next-note", "prev-note") + ") to resolve it"
		return
	}
	n := m.notes[m.rows[m.curRow].note-1]
	n.Resolved = !n.Resolved
	if err := sidecar.Save(); err != nil {
		m.status = "error: " + err.Error()
	} else if n.Resolved {
		m.status = "✓ resolved"
	} else {
		m.status = "note reopened"
	}
	m.recompute()
}

// noteRows lists the row indices of note rows, in view order.
func (m *model) noteRows() []int {
	var out []int
	for i, r := range m.rows {
		if r.note > 0 {
			out = append(out, i)
		}
	}
	return out
}

// gotoNote moves the cursor to the next (delta 1) or previous note row,
// wrapping around, and centers it.
func (m *model) gotoNote(delta int) {
	rows := m.noteRows()
	if len(rows) == 0 {
		m.status = "no notes in this file"
		if notesHidden && len(m.notes) > 0 {
			m.status = "notes are hidden (" + keys.file.first("notes-toggle") + " shows them)"
		}
		return
	}
	target := -1
	if delta > 0 {
		for _, ri := range rows {
			if ri > m.curRow {
				target = ri
				break
			}
		}
		if target < 0 {
			target = rows[0]
		}
	} else {
		for i := len(rows) - 1; i >= 0; i-- {
			if rows[i] < m.curRow {
				target = rows[i]
				break
			}
		}
		if target < 0 {
			target = rows[len(rows)-1]
		}
	}
	m.setCursor(target)
	m.top = clamp(target-m.bodyH()/3, 0, m.maxTop())
}

// startNote opens the composer for the cursor row: a new note on a code
// line, an edit of your own note, or a reply (a new note on the same anchor)
// to an agent's.
func (m *model) startNote() {
	if sidecar.Path == "" {
		m.status = "start with -notes <file.json> to take notes"
		return
	}
	if len(m.rows) == 0 {
		return
	}
	if notesHidden { // the composer needs the note rows in place
		m.toggleNotes()
	}
	r := m.rows[m.curRow]
	m.noteEdit, m.noteText, m.noteRow, m.noteEnd = nil, "", m.curRow, 0
	switch {
	case r.note > 0:
		n := m.notes[r.note-1]
		if n.Author == "human" {
			m.noteEdit, m.noteText = n, n.Summary
		}
		m.noteAnchor = [2]int{n.NewLine, n.OldLine}
		// a reply is drawn above the code line, i.e. below the existing notes
		for m.noteRow+1 < len(m.rows) && m.rows[m.noteRow].note > 0 {
			m.noteRow++
		}
	case r.fold > 0:
		m.status = "click the fold to expand it, then annotate a line"
		return
	default:
		m.noteAnchor = [2]int{r.r + 1, r.l + 1}
		if r.r < 0 { // deletion: only the left line exists
			m.noteAnchor[0] = 0
		}
	}
	m.noteInput = true
}

// toggleNotes shows or hides the note boxes, keeping the cursor on the
// same code line.
func (m *model) toggleNotes() {
	keep := row{l: -2, r: -2} // the code row at or after the cursor
	for i := m.curRow; i < len(m.rows); i++ {
		if m.rows[i].note == 0 {
			keep = m.rows[i]
			break
		}
	}
	notesHidden = !notesHidden
	m.recompute()
	for i, r := range m.rows {
		if r.note == 0 && r.l == keep.l && r.r == keep.r {
			m.setCursor(i)
			break
		}
	}
	m.status = "notes " + map[bool]string{true: "hidden", false: "shown"}[notesHidden]
}

// startRangeNote opens the composer for the visually selected rows: the
// note anchors on the first line and ends on the last, on the right side
// when the selection has any right-hand lines.
func (m *model) startRangeNote(lo, hi int) {
	m.visual = false
	m.curRow = lo
	m.startNote()
	if !m.noteInput {
		return
	}
	right := m.noteAnchor[0] > 0
	for i := hi; i > lo; i-- {
		if r := m.rows[i]; r.note == 0 {
			if right && r.r >= 0 {
				m.noteEnd = r.r + 1
				return
			}
			if !right && r.l >= 0 {
				m.noteEnd = r.l + 1
				return
			}
		}
	}
}

// saveNote stores the draft (new or edited) and writes the sidecar.
func (m *model) saveNote() {
	m.noteInput = false
	text := strings.TrimSpace(m.noteText)
	if text == "" {
		m.status = "empty note discarded"
		return
	}
	if m.noteEdit != nil {
		m.noteEdit.Summary = text
	} else {
		n := notes.Note{FilePath: m.notePath(), Summary: text, Author: "human"}
		if m.noteAnchor[0] > 0 {
			n.NewLine = m.noteAnchor[0]
		} else {
			n.OldLine = m.noteAnchor[1]
		}
		if m.noteEnd > max(n.NewLine, n.OldLine) {
			n.EndLine = m.noteEnd
		}
		// remember the line's text so the note follows later edits
		lines, ln := m.right, n.NewLine
		if n.NewLine == 0 {
			lines, ln = m.left, n.OldLine
		}
		if ln > 0 && ln <= len(lines) && notes.FindLine(lines, lines[ln-1]) == ln-1 {
			n.Match = strings.TrimSpace(lines[ln-1])
		}
		sidecar.Items = append(sidecar.Items, n)
	}
	if err := sidecar.Save(); err != nil {
		m.status = "error: " + err.Error()
	} else {
		m.status = "✎ note saved to " + sidecar.Path
	}
	m.loadFileNotes()
	m.recompute()
}

// deleteNote removes the note under the cursor from the sidecar.
func (m *model) deleteNote() {
	if len(m.rows) == 0 || m.rows[m.curRow].note == 0 {
		m.status = "move onto a note (" + hint(keys.file, "next-note", "prev-note") + ") to delete it"
		return
	}
	target := m.notes[m.rows[m.curRow].note-1]
	for i := range sidecar.Items {
		if &sidecar.Items[i] == target {
			sidecar.Items = append(sidecar.Items[:i], sidecar.Items[i+1:]...)
			break
		}
	}
	if err := sidecar.Save(); err != nil {
		m.status = "error: " + err.Error()
	} else {
		m.status = "note deleted"
	}
	m.loadFileNotes()
	m.recompute()
}

// noteStyle is the border color of a note: accent for agents, the applied
// color for yours.
func noteStyle(n *notes.Note) lipgloss.Style {
	if n.Author == "human" {
		return styleNoteHuman
	}
	return styleNote
}

// noteTitle names a note for its box: who wrote it and which line it is on.
func noteTitle(n *notes.Note) string {
	who := "note"
	switch n.Author {
	case "":
	case "human":
		who = "your note"
	default:
		who = n.Author + " note"
	}
	start, right := n.Line()
	if start == 0 {
		if n.Match != "" {
			return who + " · text not found"
		}
		return who + " · file"
	}
	where := fmt.Sprintf("L%d", start)
	if !right {
		where = "old " + where
	}
	if n.EndLine > start {
		where += fmt.Sprintf("-%d", n.EndLine)
	}
	return who + " · " + where
}

// noteBox draws a rounded frame across w cells with the title in the top
// border and hint in the bottom one, the body wrapped inside.
func noteBox(title, body, hint string, border, text lipgloss.Style, w int) []string {
	inner := max(1, w-5) // " │ " + text + " │"
	// a labelled border is " ╭─ " + label + " " + dashes + "╮": w-6-label cells of dashes
	labelled := func(l, r, label string) string {
		label = runewidth.Truncate(label, max(0, w-6), "…")
		return " " + l + "─ " + label + " " + strings.Repeat("─", max(0, w-6-lipgloss.Width(label))) + r
	}
	out := []string{border.Render(labelled("╭", "╮", title))}
	for _, l := range strings.Split(lipgloss.NewStyle().Width(inner).Render(body), "\n") {
		out = append(out, border.Render(" │ ")+text.Render(l)+border.Render(" │"))
	}
	bottom := " ╰" + strings.Repeat("─", max(0, w-3)) + "╯"
	if hint != "" {
		bottom = labelled("╰", "╯", hint)
	}
	return append(out, border.Render(bottom))
}

// noteLines renders note row r across w cells as a box; cur highlights it.
// A resolved note collapses to one dim line with the start of its text.
func (m *model) noteLines(r row, w int, cur bool) []string {
	n := m.notes[r.note-1]
	if n.Resolved {
		first, _, _ := strings.Cut(n.Summary, "\n")
		label := " ✓ " + noteTitle(n) + " · " + first
		st := styleFold
		if cur {
			st = styleNoteTextCur
		}
		return []string{st.Render(runewidth.Truncate(label, max(1, w-1), "…") +
			strings.Repeat(" ", max(0, w-1-runewidth.StringWidth(label))))}
	}
	border, text := styleNote, styleNoteText
	if n.Author == "human" {
		border = styleNoteHuman
	}
	if cur {
		border, text = border.Bold(true), styleNoteTextCur
	}
	return noteBox(noteTitle(n), n.Summary, "", border, text, w)
}

// composerLines draws the note being written as a box: the draft with a
// cursor and the editing keys in the bottom border.
func (m *model) composerLines(w int) []string {
	title := "your note"
	if m.noteEdit != nil {
		title = "editing " + noteTitle(m.noteEdit)
	} else if m.noteEnd > 0 {
		title = noteTitle(&notes.Note{Author: "human", NewLine: m.noteAnchor[0], OldLine: m.noteAnchor[1], EndLine: m.noteEnd})
	}
	return noteBox(title, m.noteText+"▏", "ctrl+s save · enter new line · esc cancel",
		styleNoteHuman.Bold(true), styleNoteTextCur, w)
}
