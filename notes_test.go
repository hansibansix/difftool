package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// noteModel builds a model whose notes come from items (store path is
// set so the feature is on without touching disk).
func noteModel(t *testing.T, left, right []string, items ...note) *model {
	t.Helper()
	notes.path, notes.items = filepath.Join(t.TempDir(), "notes.json"), items
	t.Cleanup(func() { notes.path, notes.items = "", nil })
	m := &model{left: left, right: right, savedL: left, savedR: right, leftNL: true, rightNL: true, w: 80, h: 5, noteKey: "f.txt"}
	m.loadFileNotes()
	m.recompute()
	return m
}

func TestNoteMatches(t *testing.T) {
	for _, c := range []struct {
		f, key string
		want   bool
	}{
		{"a/b.txt", "a/b.txt", true},
		{"b.txt", "proj/a/b.txt", true},   // agent path shorter than ours
		{"repo/a/b.txt", "a/b.txt", true}, // agent path longer than ours
		{"ab.txt", "a/b.txt", false},
		{"", "a/b.txt", false},
	} {
		if got := noteMatches(c.f, c.key); got != c.want {
			t.Errorf("noteMatches(%q, %q) = %v", c.f, c.key, got)
		}
	}
}

func TestNoteRowsPlacement(t *testing.T) {
	m := noteModel(t, []string{"a", "b", "c", "d"}, []string{"a", "B", "c", "d"},
		note{FilePath: "f.txt", NewLine: 3, Summary: "on c"},
		note{FilePath: "f.txt", Summary: "file level"},
		note{FilePath: "f.txt", OldLine: 2, Summary: "on old b"},
		note{FilePath: "other.txt", NewLine: 1, Summary: "elsewhere"},
	)
	if len(m.notes) != 3 {
		t.Fatalf("notes for f.txt = %d", len(m.notes))
	}
	var got []string
	for _, r := range m.rows {
		switch {
		case r.note > 0:
			got = append(got, m.notes[r.note-1].Summary)
		case r.r >= 0:
			got = append(got, m.right[r.r])
		default:
			got = append(got, "-"+m.left[r.l])
		}
	}
	want := []string{"file level", "a", "on old b", "B", "on c", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows = %v, want %v", got, want)
		}
	}
	if len(m.nav) != 1 || m.rows[m.nav[0].row].r != 1 {
		t.Fatalf("nav must point at the B row after insertion: %+v", m.nav)
	}
}

func TestNotesShiftWithApplyAndUndo(t *testing.T) {
	m := noteModel(t, []string{"a", "b", "c", "d"}, []string{"a", "X", "Y", "c", "d"},
		note{FilePath: "f.txt", NewLine: 4, Summary: "on c"},
		note{FilePath: "f.txt", NewLine: 2, Summary: "inside the hunk"},
	)
	m.apply(true) // X,Y -> b: right shrinks by one
	if m.notes[0].NewLine != 3 || m.notes[1].NewLine != 2 {
		t.Fatalf("anchors after apply: %d %d", m.notes[0].NewLine, m.notes[1].NewLine)
	}
	for i, r := range m.rows {
		if r.note == 1 { // the "on c" note
			if next := m.rows[i+1]; next.r < 0 || m.right[next.r] != "c" {
				t.Fatalf("note must still introduce line c, got row %+v", next)
			}
		}
	}
	m.undoLast()
	if m.notes[0].NewLine != 4 {
		t.Fatalf("undo must restore the anchor, got %d", m.notes[0].NewLine)
	}
	m.applyAll(false) // left takes X,Y: left grows, right untouched
	if m.notes[0].NewLine != 4 {
		t.Fatalf("applying to the left must not move right-side anchors: %d", m.notes[0].NewLine)
	}
}

func TestFoldKeepsNotedLine(t *testing.T) {
	cfg.Fold = true
	defer func() { cfg.Fold = false }()
	left := seq(0, 30)
	right := append(append(append([]string{}, left[:10]...), "CHANGED"), left[11:]...)
	m := noteModel(t, left, right, note{FilePath: "f.txt", NewLine: 25, Summary: "deep in the fold"})
	for i, r := range m.rows {
		if r.note > 0 {
			if next := m.rows[i+1]; next.fold > 0 || next.r != 24 {
				t.Fatalf("note must sit above its unfolded line, got %+v", next)
			}
			return
		}
	}
	t.Fatal("note row missing")
}

func TestVisualSkipsNoteRows(t *testing.T) {
	m := noteModel(t, []string{"a", "b", "c", "d", "e"}, []string{"a", "B", "C", "D", "e"},
		note{FilePath: "f.txt", NewLine: 2, Summary: "on B"})
	first, last := m.chunkRows() // rows: a, note, B, C, D, e
	if first != 2 || last != 4 || m.rows[last].r != 3 {
		t.Fatalf("chunkRows = %d..%d (%+v)", first, last, m.rows[last])
	}
	m.visual, m.vAnchor, m.vCur = true, first, first
	m.moveVisual(1, first, last)
	if m.rows[m.vCur].note > 0 || m.rows[m.vCur].r != 2 {
		t.Fatalf("visual cursor must skip the note row: %+v", m.rows[m.vCur])
	}
	m.applySelection(true) // B,C selected via rows 2..3 -> two lines
	want := []string{"a", "b", "c", "D", "e"}
	for i := range want {
		if m.right[i] != want[i] {
			t.Fatalf("right = %v", m.right)
		}
	}
}

func TestAddNoteWritesSidecarAndReloads(t *testing.T) {
	m := noteModel(t, []string{"a", "b"}, []string{"a", "B"})
	m.setCursor(1) // the B row
	m.startNote()
	if !m.noteInput || m.noteAnchor != [2]int{2, 2} || m.noteEdit != nil {
		t.Fatalf("input=%v anchor=%v edit=%v", m.noteInput, m.noteAnchor, m.noteEdit)
	}
	m.noteText = "  use get_string here "
	m.saveNote()
	data, err := os.ReadFile(notes.path)
	if err != nil {
		t.Fatal(err)
	}
	var f noteFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Comments) != 1 || f.Comments[0] != (note{FilePath: "f.txt", NewLine: 2, Summary: "use get_string here", Author: "human"}) {
		t.Fatalf("sidecar = %+v", f.Comments)
	}
	if len(m.notes) != 1 || m.noteRows() == nil {
		t.Fatalf("note must show right away: %d", len(m.notes))
	}
	if reloadNotes() {
		t.Fatal("our own write must not count as an external change")
	}
	// an agent rewrites the file: picked up by mtime
	f.Comments = append(f.Comments, note{FilePath: "f.txt", NewLine: 1, Summary: "agent reply"})
	data, _ = json.Marshal(f)
	os.WriteFile(notes.path, data, 0o644)
	os.Chtimes(notes.path, time.Now(), notes.mtime.Add(2*time.Second))
	if !reloadNotes() || len(notes.items) != 2 {
		t.Fatalf("reload failed: %d items", len(notes.items))
	}
	m.loadFileNotes()
	m.recompute()
	if len(m.noteRows()) != 2 {
		t.Fatalf("note rows after reload = %d", len(m.noteRows()))
	}
	// rows: agent note (above a), a, our note (above B), B
	m.setCursor(0)
	if r := m.rows[0]; r.note == 0 || m.notes[r.note-1].Summary != "agent reply" {
		t.Fatalf("the agent note must sit above line 1, got %+v", r)
	}
	m.gotoNote(1) // } lands on our note; c there edits it
	m.startNote()
	if m.noteEdit == nil || m.noteText != "use get_string here" {
		t.Fatalf("c on an own note must edit it: edit=%v text=%q", m.noteEdit, m.noteText)
	}
	m.noteText = "use get_string here\nsecond line"
	m.saveNote()
	if notes.items[0].Summary != "use get_string here\nsecond line" || len(notes.items) != 2 {
		t.Fatalf("edit must rewrite in place: %+v", notes.items)
	}
	// delete the note under the cursor
	m.deleteNote()
	if len(notes.items) != 1 || len(m.noteRows()) != 1 {
		t.Fatalf("delete: %d items, %d rows", len(notes.items), len(m.noteRows()))
	}
	// c on an agent note replies: a new note on the same anchor, composed
	// right above the code line
	m.gotoNote(1)
	m.startNote()
	if m.noteEdit != nil || m.noteAnchor != [2]int{1, 0} || m.rows[m.noteRow].r != 0 {
		t.Fatalf("reply: edit=%v anchor=%v row=%+v", m.noteEdit, m.noteAnchor, m.rows[m.noteRow])
	}
}

func TestCursorMovesAndFollowsHunks(t *testing.T) {
	m := testModel(seq(0, 20), append(append(append([]string{}, seq(0, 10)...), "X"), seq(11, 20)...))
	m.h = 7 // 5 body lines
	if m.curRow != 0 || m.top != 0 {
		t.Fatalf("start: row %d top %d", m.curRow, m.top)
	}
	for i := 0; i < 6; i++ {
		m.setCursor(m.curRow + 1)
	}
	if m.curRow != 6 || m.top != 2 {
		t.Fatalf("cursor must drag the view: row %d top %d", m.curRow, m.top)
	}
	m.setCursor(10) // the changed line
	if m.nav[m.cur].row != 10 {
		t.Fatalf("current hunk must follow the cursor: %+v", m.nav[m.cur])
	}
	m.setCursor(0)
	if m.top != 0 {
		t.Fatalf("scrolling up: top %d", m.top)
	}
}

func TestNoteOnDeletionAnchorsOldLine(t *testing.T) {
	m := noteModel(t, []string{"a", "gone", "b"}, []string{"a", "b"})
	m.setCursor(1) // the deleted line
	m.startNote()
	m.noteText = "why removed?"
	m.saveNote()
	if n := notes.items[0]; n.NewLine != 0 || n.OldLine != 2 {
		t.Fatalf("deletion note must anchor on the old line: %+v", n)
	}
}

func TestRangeNoteFromVisualAndShift(t *testing.T) {
	m := noteModel(t, []string{"a", "b", "c", "d", "e"}, []string{"a", "B", "C", "D", "e"})
	m.setCursor(1) // B
	m.visual, m.vAnchor, m.vCur = true, 1, 3
	m.startRangeNote(1, 3)
	if !m.noteInput || m.noteAnchor != [2]int{2, 2} || m.noteEnd != 4 {
		t.Fatalf("range composer: anchor=%v end=%d", m.noteAnchor, m.noteEnd)
	}
	m.noteText = "these three"
	m.saveNote()
	n := notes.items[0]
	if n.NewLine != 2 || n.EndLine != 4 || !strings.Contains(noteTitle(&n), "L2-4") {
		t.Fatalf("saved range: %+v title %q", n, noteTitle(&n))
	}
	// covered lines are tinted, others not; the left side never is
	if m.noteAt(1, true) == nil || m.noteAt(3, true) == nil || m.noteAt(4, true) != nil || m.noteAt(2, false) != nil {
		t.Fatal("covers() must span exactly lines 2-4 on the right")
	}
	// the note sits above line 2 only
	if rows := m.noteRows(); len(rows) != 1 || m.rows[rows[0]+1].r != 1 {
		t.Fatalf("range note must anchor once, above its first line: %v", rows)
	}
	// applying a hunk above shifts start and end together; undo restores both
	m2 := noteModel(t, []string{"a", "b", "c", "d"}, []string{"a", "X", "Y", "c", "d"},
		note{FilePath: "f.txt", NewLine: 4, EndLine: 5, Summary: "c-d"})
	m2.apply(true)
	if m2.notes[0].NewLine != 3 || m2.notes[0].EndLine != 4 {
		t.Fatalf("shift: %+v", m2.notes[0])
	}
	m2.undoLast()
	if m2.notes[0].NewLine != 4 || m2.notes[0].EndLine != 5 {
		t.Fatalf("undo: %+v", m2.notes[0])
	}
}

func TestResolveToggle(t *testing.T) {
	m := noteModel(t, []string{"a", "b"}, []string{"a", "B"},
		note{FilePath: "f.txt", NewLine: 2, Summary: "check this\nsecond line"})
	m.gotoNote(1)
	if len(m.noteLines(m.rows[m.curRow], 60, false)) < 3 {
		t.Fatal("an open note renders as a box")
	}
	m.toggleResolved()
	n := m.notes[0]
	if !n.Resolved || len(m.noteLines(m.rows[m.curRow], 60, false)) != 1 || noteCount("f.txt") != 0 {
		t.Fatalf("resolved: %+v lines=%d open=%d", n, len(m.noteLines(m.rows[m.curRow], 60, false)), noteCount("f.txt"))
	}
	if m.noteAt(1, true) != nil {
		t.Fatal("resolved notes must not tint line numbers")
	}
	data, _ := os.ReadFile(notes.path)
	if !strings.Contains(string(data), `"resolved": true`) {
		t.Fatalf("resolved flag must be persisted: %s", data)
	}
	m.toggleResolved()
	if m.notes[0].Resolved {
		t.Fatal("toggle must reopen")
	}
}

func TestNoteBoxWidth(t *testing.T) {
	for _, w := range []int{20, 57, 108} {
		for _, hint := range []string{"", "ctrl+s save · esc cancel"} {
			lines := noteBox("your note · L42-47", "some text that is long enough to wrap around in a narrow box", hint,
				lipgloss.NewStyle(), lipgloss.NewStyle(), w)
			for i, l := range lines {
				if lipgloss.Width(l) != w {
					t.Fatalf("w=%d hint=%q line %d is %d wide: %q", w, hint, i, lipgloss.Width(l), l)
				}
			}
		}
	}
}

func TestReloadWhenFileChangesOnDisk(t *testing.T) {
	dir := t.TempDir()
	l, r := filepath.Join(dir, "l.txt"), filepath.Join(dir, "r.txt")
	os.WriteFile(l, []byte("a\nb\n"), 0o644)
	os.WriteFile(r, []byte("a\nB\n"), 0o644)
	m, err := newModel(l, r)
	if err != nil {
		t.Fatal(err)
	}
	if m.checkDisk() {
		t.Fatal("nothing changed yet")
	}
	// an agent rewrites the right file
	os.WriteFile(r, []byte("a\nB\nnew\n"), 0o644)
	os.Chtimes(r, time.Now(), m.rightMod.Add(2*time.Second))
	if !m.checkDisk() || len(m.right) != 3 || !strings.Contains(m.status, "reloaded") {
		t.Fatalf("reload: right=%v status=%q", m.right, m.status)
	}
	if m.checkDisk() {
		t.Fatal("a reload must stamp the new mtime")
	}
	// unsaved changes block the reload and are reported once
	m.apply(true)
	os.WriteFile(r, []byte("a\nB\nnewer\n"), 0o644)
	os.Chtimes(r, time.Now(), m.rightMod.Add(2*time.Second))
	if m.checkDisk() || !m.dirty() || !strings.Contains(m.status, "changed on disk") {
		t.Fatalf("dirty view must not reload: status=%q", m.status)
	}
	m.status = ""
	if m.checkDisk() || m.status != "" {
		t.Fatal("the warning must not repeat every tick")
	}
	m.undoLast()
	if !m.checkDisk() || m.right[2] != "newer" {
		t.Fatalf("after undo the reload goes through: %v", m.right)
	}
}

func TestNotesToggle(t *testing.T) {
	m := noteModel(t, []string{"a", "b", "c"}, []string{"a", "B", "c"},
		note{FilePath: "f.txt", NewLine: 2, Summary: "on B"}, note{FilePath: "f.txt", NewLine: 3, Summary: "on c"})
	defer func() { notesHidden = false }()
	m.setCursor(2) // B, below the first note row
	m.toggleNotes()
	if !notesHidden || len(m.noteRows()) != 0 || m.rows[m.curRow].r != 1 {
		t.Fatalf("hidden: rows=%d cursor=%+v", len(m.noteRows()), m.rows[m.curRow])
	}
	if m.noteAt(1, true) == nil {
		t.Fatal("the gutter tint stays while hidden")
	}
	m.toggleNotes()
	if notesHidden || len(m.noteRows()) != 2 || m.rows[m.curRow].r != 1 {
		t.Fatalf("shown again: rows=%d cursor=%+v", len(m.noteRows()), m.rows[m.curRow])
	}
	m.toggleNotes()
	m.startNote() // composing needs the rows, so it shows them
	if notesHidden || !m.noteInput {
		t.Fatal("c must un-hide the notes")
	}
}
