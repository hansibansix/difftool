package main

import (
	"reflect"
	"testing"
)

func testModel(left, right []string) *model {
	m := &model{left: left, right: right, leftNL: true, rightNL: true, w: 80, h: 5}
	m.recompute()
	return m
}

func TestApplyRecordsRegionAndKeepsView(t *testing.T) {
	m := testModel(
		[]string{"a", "b", "c", "d"},
		[]string{"a", "X", "Y", "c", "d2"},
	)
	m.top = 1
	m.apply(true) // b <-> X,Y: right becomes a,b,c,d2
	if m.top != 1 {
		t.Fatalf("view jumped: top=%d", m.top)
	}
	want := []appliedRegion{{1, 2, 1, 2, true, []string{"X", "Y"}}}
	if !reflect.DeepEqual(m.applied, want) {
		t.Fatalf("applied = %+v, want %+v", m.applied, want)
	}
	if ai, toRight := m.appliedAt(row{l: 1, r: 1}); ai < 0 || !toRight {
		t.Fatalf("appliedAt(1) = %v %v", ai, toRight)
	}
	if ai, _ := m.appliedAt(row{l: 0, r: 0}); ai >= 0 {
		t.Fatal("row 0 must not be marked applied")
	}
}

func TestApplyShiftsEarlierMarkers(t *testing.T) {
	m := testModel(
		[]string{"a", "b", "c", "d"},
		[]string{"a", "X", "Y", "c", "D"},
	)
	m.apply(true) // first change: right rows shrink by 1 after row 2
	// markers after a later apply on the d/D chunk must stay consistent
	m.cur = 1 // the applied region keeps position 0 in the nav list
	m.apply(true)
	want := []appliedRegion{
		{1, 2, 1, 2, true, []string{"X", "Y"}},
		{3, 4, 3, 4, true, []string{"D"}},
	}
	if !reflect.DeepEqual(m.applied, want) {
		t.Fatalf("applied = %+v, want %+v", m.applied, want)
	}
	if !reflect.DeepEqual(m.left, m.right) {
		t.Fatalf("files should be equal, left=%v right=%v", m.left, m.right)
	}
}

func TestApplyShiftMovesLaterMarker(t *testing.T) {
	m := testModel(
		[]string{"a", "b", "c", "d"},
		[]string{"a", "B", "c", "X", "Y", "d"},
	)
	// apply second change first (X,Y deletion), then the first (b/B):
	// lengths before the second marker don't change, so it stays put
	m.cur = 1
	m.apply(true) // removes X,Y from right -> marker {3,3,3,3}? empty region, skipped is fine
	m.cur = 0
	m.apply(true)
	for _, a := range m.applied {
		if a.l1 > a.l0 { // non-empty regions must contain equal lines
			for i := 0; i < a.l1-a.l0; i++ {
				if m.left[a.l0+i] != m.right[a.r0+i] {
					t.Fatalf("marker %+v points at unequal lines", a)
				}
			}
		}
	}
}

func TestUndoRestoresMarkers(t *testing.T) {
	m := testModel([]string{"a", "b"}, []string{"a", "B"})
	m.apply(false)
	if len(m.applied) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(m.applied))
	}
	m.undoLast()
	if len(m.applied) != 0 {
		t.Fatalf("undo must drop marker, got %+v", m.applied)
	}
}

// applied regions must be jumpable with n/p and refuse re-apply
func TestNavIncludesApplied(t *testing.T) {
	m := testModel(
		[]string{"a", "b", "c", "d", "e"},
		[]string{"a", "B", "c", "D", "e"},
	)
	m.apply(true)
	if len(m.nav) != 2 {
		t.Fatalf("nav = %+v", m.nav)
	}
	if m.nav[0].ai != 0 || m.nav[0].ci != -1 {
		t.Fatalf("first target should be the applied region: %+v", m.nav[0])
	}
	if m.nav[1].ci < 0 {
		t.Fatalf("second target should be the pending change: %+v", m.nav[1])
	}
	if m.cur != 0 {
		t.Fatalf("cursor should stay on the applied hunk, cur=%d", m.cur)
	}
	undos := len(m.undo)
	m.apply(true)
	if len(m.undo) != undos || m.status != "chunk already applied" {
		t.Fatalf("re-apply must be refused: %q", m.status)
	}
}

func TestResetApplied(t *testing.T) {
	left := []string{"a", "b", "c", "d"}
	right := []string{"a", "X", "Y", "c", "D"}
	m := testModel(append([]string(nil), left...), append([]string(nil), right...))
	m.apply(true) // b <-> X,Y
	m.cur = 1
	m.apply(true) // d <-> D
	// reset the first applied hunk; cursor sits on the second, move back
	m.cur = 0
	m.resetApplied()
	if m.status != "↺ reset" {
		t.Fatalf("status %q", m.status)
	}
	wantRight := []string{"a", "X", "Y", "c", "d"}
	if !reflect.DeepEqual(m.right, wantRight) {
		t.Fatalf("right = %v want %v", m.right, wantRight)
	}
	if len(m.applied) != 1 {
		t.Fatalf("applied = %+v", m.applied)
	}
	// the remaining marker must still point at equal lines
	a := m.applied[0]
	for i := 0; i < a.l1-a.l0; i++ {
		if m.left[a.l0+i] != m.right[a.r0+i] {
			t.Fatalf("marker %+v points at unequal lines", a)
		}
	}
	// hunk is a pending change again at nav position 0
	if m.nav[0].ci < 0 {
		t.Fatalf("reset hunk should be pending again: %+v", m.nav)
	}
	// undo restores the applied state
	m.undoLast()
	if len(m.applied) != 2 || !reflect.DeepEqual(m.right[1], "b") {
		t.Fatalf("undo after reset: applied=%d right=%v", len(m.applied), m.right)
	}
}

func TestResetRefusedOnPendingChange(t *testing.T) {
	m := testModel([]string{"a"}, []string{"b"})
	m.resetApplied()
	if m.status != "not on an applied chunk" || m.dirtyL || m.dirtyR {
		t.Fatalf("reset on pending change must be refused: %q", m.status)
	}
}
