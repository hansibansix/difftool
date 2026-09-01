package main

import (
	"reflect"
	"testing"
)

// chunksValid checks that chunks partition both files contiguously and that
// equal chunks really contain equal lines.
func chunksValid(t *testing.T, a, b []string, chunks []chunk) {
	t.Helper()
	la, lb := 0, 0
	for _, c := range chunks {
		if c.l0 != la || c.r0 != lb {
			t.Fatalf("non-contiguous chunk %+v (expected l0=%d r0=%d)", c, la, lb)
		}
		if c.l1 < c.l0 || c.r1 < c.r0 {
			t.Fatalf("negative range %+v", c)
		}
		if c.kind == kindEqual {
			if c.l1-c.l0 != c.r1-c.r0 {
				t.Fatalf("unbalanced equal chunk %+v", c)
			}
			for i := 0; i < c.l1-c.l0; i++ {
				if a[c.l0+i] != b[c.r0+i] {
					t.Fatalf("equal chunk %+v has differing lines", c)
				}
			}
		}
		la, lb = c.l1, c.r1
	}
	if la != len(a) || lb != len(b) {
		t.Fatalf("chunks do not cover files: got %d/%d, want %d/%d", la, lb, len(a), len(b))
	}
}

func TestDiffChunksModification(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "x", "c"}
	got := diffChunks(a, b)
	want := []chunk{
		{kindEqual, 0, 1, 0, 1},
		{kindChange, 1, 2, 1, 2},
		{kindEqual, 2, 3, 2, 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	chunksValid(t, a, b, got)
}

func TestDiffChunksInsertDelete(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "c", "x", "d"}
	got := diffChunks(a, b)
	chunksValid(t, a, b, got)
	var changes int
	for _, c := range got {
		if c.kind == kindChange {
			changes++
		}
	}
	if changes != 2 {
		t.Fatalf("want 2 change chunks, got %d: %+v", changes, got)
	}
}

func TestDiffChunksIdentical(t *testing.T) {
	a := []string{"a", "b"}
	got := diffChunks(a, a)
	want := []chunk{{kindEqual, 0, 2, 0, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestDiffChunksEmptySides(t *testing.T) {
	chunksValid(t, nil, nil, diffChunks(nil, nil))
	a := []string{"a", "b"}
	chunksValid(t, a, nil, diffChunks(a, nil))
	chunksValid(t, nil, a, diffChunks(nil, a))
}

func TestDiffChunksDisjoint(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"x", "y", "z"}
	got := diffChunks(a, b)
	want := []chunk{{kindChange, 0, 2, 0, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestMyersFallbackLargeDistance(t *testing.T) {
	// two files with no common lines beyond the cap: must not blow up,
	// middle becomes one change chunk
	var a, b []string
	for i := 0; i < 3000; i++ {
		a = append(a, "a"+string(rune('0'+i%10))+itoa(i))
		b = append(b, "b"+string(rune('0'+i%10))+itoa(i))
	}
	got := diffChunks(a, b)
	chunksValid(t, a, b, got)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// Applying every change chunk left→right must turn right into left.
func TestApplyAllMakesEqual(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e", "f"}
	b := []string{"a", "x", "c", "e", "y", "f", "g"}
	right := append([]string{}, b...)
	for {
		chunks := diffChunks(a, right)
		var c *chunk
		for i := range chunks {
			if chunks[i].kind == kindChange {
				c = &chunks[i]
				break
			}
		}
		if c == nil {
			break
		}
		right = splice(right, c.r0, c.r1, a[c.l0:c.l1])
	}
	if !reflect.DeepEqual(right, a) {
		t.Fatalf("got %v want %v", right, a)
	}
}

func TestSplice(t *testing.T) {
	d := []string{"a", "b", "c"}
	if got := splice(d, 1, 2, []string{"x", "y"}); !reflect.DeepEqual(got, []string{"a", "x", "y", "c"}) {
		t.Fatalf("got %v", got)
	}
	if got := splice(d, 0, 3, nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := splice(d, 3, 3, []string{"z"}); !reflect.DeepEqual(got, []string{"a", "b", "c", "z"}) {
		t.Fatalf("got %v", got)
	}
}
