package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Agent notes: a sidecar JSON file in hunk's --agent-context format
// (https://github.com/modem-dev/hunk) anchoring remarks to lines of the
// compared files. The file is polled so an agent can add notes while the
// diff is open, and notes typed in difftool are written back to it with
// author "human" for the agent to read.

type note struct {
	FilePath string `json:"filePath"`
	NewLine  int    `json:"newLine,omitempty"` // 1-based line of the right side
	OldLine  int    `json:"oldLine,omitempty"` // 1-based line of the left side
	Summary  string `json:"summary"`
	Author   string `json:"author,omitempty"`
}

type noteFile struct {
	Comments []note `json:"comments"`
}

// notes is the loaded sidecar; empty path = feature off.
var notes struct {
	path  string
	items []note
	mtime time.Time
}

// notesTickMsg drives the poll of the sidecar file.
type notesTickMsg struct{}

const notesPoll = time.Second

func notesTick() tea.Cmd {
	if notes.path == "" {
		return nil
	}
	return tea.Tick(notesPoll, func(time.Time) tea.Msg { return notesTickMsg{} })
}

// loadNotes reads the sidecar; a missing file is an empty set (it is
// created by the first note typed in difftool).
func loadNotes(path string) error {
	notes.path, notes.items, notes.mtime = path, nil, time.Time{}
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f noteFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	notes.items, notes.mtime = f.Comments, st.ModTime()
	return nil
}

// reloadNotes re-reads the sidecar when its mtime changed and reports
// whether it did.
func reloadNotes() bool {
	st, err := os.Stat(notes.path)
	if err != nil || st.ModTime().Equal(notes.mtime) {
		return false
	}
	return loadNotes(notes.path) == nil
}

func saveNotes() error {
	data, err := json.MarshalIndent(noteFile{notes.items}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileMkdir(notes.path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if st, err := os.Stat(notes.path); err == nil {
		notes.mtime = st.ModTime() // our own write is not a reload
	}
	return nil
}

// noteMatches reports whether a note's filePath names key. Agents write
// paths relative to the repository or project root while the key is what
// difftool knows (repo-relative in git mode, root-relative in dir mode,
// cwd-relative for two files), so either may carry extra leading dirs.
func noteMatches(filePath, key string) bool {
	return filePath == key || strings.HasSuffix(key, "/"+filePath) || strings.HasSuffix(filePath, "/"+key)
}

// notesFor returns pointers to the notes of one file, in file order.
func notesFor(key string) []*note {
	var out []*note
	for i := range notes.items {
		if noteMatches(notes.items[i].FilePath, key) {
			out = append(out, &notes.items[i])
		}
	}
	return out
}

func noteCount(key string) int { return len(notesFor(key)) }

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
	if notes.path != "" {
		m.notes = notesFor(m.notePath())
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
	if len(m.notes) == 0 {
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

// shiftNotes moves anchors at or after `from` on one side by delta lines,
// mirroring shiftApplied, so notes follow an applied or reset hunk.
func (m *model) shiftNotes(right bool, from, delta int) {
	for _, n := range m.notes {
		if right && n.NewLine > from {
			n.NewLine += delta
		}
		if !right && n.OldLine > from {
			n.OldLine += delta
		}
	}
}

// noteAnchors snapshots the anchors for undo.
func (m *model) noteAnchors() [][2]int {
	out := make([][2]int, len(m.notes))
	for i, n := range m.notes {
		out[i] = [2]int{n.NewLine, n.OldLine}
	}
	return out
}

func (m *model) restoreAnchors(a [][2]int) {
	if len(a) != len(m.notes) {
		return // the sidecar changed meanwhile; keep what it says
	}
	for i, n := range m.notes {
		n.NewLine, n.OldLine = a[i][0], a[i][1]
	}
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
	if notes.path == "" {
		m.status = "start with -notes <file.json> to take notes"
		return
	}
	if len(m.rows) == 0 {
		return
	}
	r := m.rows[m.curRow]
	m.noteEdit, m.noteText, m.noteRow = nil, "", m.curRow
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
		n := note{FilePath: m.notePath(), Summary: text, Author: "human"}
		if m.noteAnchor[0] > 0 {
			n.NewLine = m.noteAnchor[0]
		} else {
			n.OldLine = m.noteAnchor[1]
		}
		notes.items = append(notes.items, n)
	}
	if err := saveNotes(); err != nil {
		m.status = "error: " + err.Error()
	} else {
		m.status = "✎ note saved to " + notes.path
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
	for i := range notes.items {
		if &notes.items[i] == target {
			notes.items = append(notes.items[:i], notes.items[i+1:]...)
			break
		}
	}
	if err := saveNotes(); err != nil {
		m.status = "error: " + err.Error()
	} else {
		m.status = "note deleted"
	}
	m.loadFileNotes()
	m.recompute()
}

// noteTitle names a note for its box: who wrote it and which line it is on.
func noteTitle(n *note) string {
	who := "note"
	switch n.Author {
	case "":
	case "human":
		who = "your note"
	default:
		who = n.Author + " note"
	}
	switch {
	case n.NewLine > 0:
		return fmt.Sprintf("%s · L%d", who, n.NewLine)
	case n.OldLine > 0:
		return fmt.Sprintf("%s · old L%d", who, n.OldLine)
	}
	return who + " · file"
}

// noteBox draws a rounded frame across w cells with the title in the top
// border and hint in the bottom one, the body wrapped inside.
func noteBox(title, body, hint string, border, text lipgloss.Style, w int) []string {
	inner := max(1, w-5) // " ╭" + "│ " … " │"
	fill := func(s string) string { return strings.Repeat("─", max(0, inner+2-lipgloss.Width(s))) }
	out := []string{border.Render(" ╭─ " + title + " " + fill(" "+title+" ") + "╮")}
	for _, l := range strings.Split(lipgloss.NewStyle().Width(inner).Render(body), "\n") {
		out = append(out, border.Render(" │ ")+text.Render(l)+border.Render(" │"))
	}
	bottom := " ╰" + fill("") + "╯"
	if hint != "" {
		bottom = " ╰─ " + hint + " " + fill(" "+hint+" ") + "╯"
	}
	return append(out, border.Render(bottom))
}

// noteLines renders note row r across w cells as a box; cur highlights it.
func (m *model) noteLines(r row, w int, cur bool) []string {
	n := m.notes[r.note-1]
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
	}
	return noteBox(title, m.noteText+"▏", "ctrl+s save · enter new line · esc cancel",
		styleNoteHuman.Bold(true), styleNoteTextCur, w)
}
