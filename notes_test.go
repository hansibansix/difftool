package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	want := []string{"file level", "a", "B", "on old b", "c", "on c", "d"}
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
			if prev := m.rows[i-1]; prev.r < 0 || m.right[prev.r] != "c" {
				t.Fatalf("note must still follow line c, got row %+v", prev)
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
			if prev := m.rows[i-1]; prev.fold > 0 || prev.r != 24 {
				t.Fatalf("note must sit below its unfolded line, got %+v", prev)
			}
			return
		}
	}
	t.Fatal("note row missing")
}

func TestVisualSkipsNoteRows(t *testing.T) {
	m := noteModel(t, []string{"a", "b", "c", "d", "e"}, []string{"a", "B", "C", "D", "e"},
		note{FilePath: "f.txt", NewLine: 2, Summary: "on B"})
	first, last := m.chunkRows()
	if first != 1 || last != 4 || m.rows[last].r != 3 {
		t.Fatalf("chunkRows = %d..%d (%+v)", first, last, m.rows[last])
	}
	m.visual, m.vAnchor, m.vCur = true, first, first
	m.moveVisual(1, first, last)
	if m.rows[m.vCur].note > 0 || m.rows[m.vCur].r != 2 {
		t.Fatalf("visual cursor must skip the note row: %+v", m.rows[m.vCur])
	}
	m.applySelection(true) // B,C selected via rows 1..3 -> two lines
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
	// } lands on the next note row after the cursor; c there edits our own note
	m.setCursor(0)
	m.gotoNote(1)
	if r := m.rows[m.curRow]; r.note == 0 || m.notes[r.note-1].Summary != "agent reply" {
		t.Fatalf("first jump must reach the note under line 1, got %+v", r)
	}
	m.gotoNote(1)
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
	// c on an agent note replies: a new note on the same anchor
	m.gotoNote(1)
	m.startNote()
	if m.noteEdit != nil || m.noteAnchor != [2]int{1, 0} {
		t.Fatalf("reply: edit=%v anchor=%v", m.noteEdit, m.noteAnchor)
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
