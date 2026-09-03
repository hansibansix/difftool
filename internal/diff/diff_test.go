package diff

import (
	"reflect"
	"testing"
)

// chunksValid checks that chunks partition both files contiguously and that
// equal chunks really contain equal lines.
func chunksValid(t *testing.T, a, b []string, chunks []Chunk) {
	t.Helper()
	la, lb := 0, 0
	for _, c := range chunks {
		if c.L0 != la || c.R0 != lb {
			t.Fatalf("non-contiguous Chunk %+v (expected l0=%d r0=%d)", c, la, lb)
		}
		if c.L1 < c.L0 || c.R1 < c.R0 {
			t.Fatalf("negative range %+v", c)
		}
		if c.Kind == Equal {
			if c.L1-c.L0 != c.R1-c.R0 {
				t.Fatalf("unbalanced equal Chunk %+v", c)
			}
			for i := 0; i < c.L1-c.L0; i++ {
				if a[c.L0+i] != b[c.R0+i] {
					t.Fatalf("equal Chunk %+v has differing lines", c)
				}
			}
		}
		la, lb = c.L1, c.R1
	}
	if la != len(a) || lb != len(b) {
		t.Fatalf("chunks do not cover files: got %d/%d, want %d/%d", la, lb, len(a), len(b))
	}
}

func TestDiffChunksModification(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "x", "c"}
	got := Chunks(a, b)
	want := []Chunk{
		{Equal, 0, 1, 0, 1},
		{Change, 1, 2, 1, 2},
		{Equal, 2, 3, 2, 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	chunksValid(t, a, b, got)
}

func TestDiffChunksInsertDelete(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "c", "x", "d"}
	got := Chunks(a, b)
	chunksValid(t, a, b, got)
	var changes int
	for _, c := range got {
		if c.Kind == Change {
			changes++
		}
	}
	if changes != 2 {
		t.Fatalf("want 2 change chunks, got %d: %+v", changes, got)
	}
}

func TestDiffChunksIdentical(t *testing.T) {
	a := []string{"a", "b"}
	got := Chunks(a, a)
	want := []Chunk{{Equal, 0, 2, 0, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestDiffChunksEmptySides(t *testing.T) {
	chunksValid(t, nil, nil, Chunks(nil, nil))
	a := []string{"a", "b"}
	chunksValid(t, a, nil, Chunks(a, nil))
	chunksValid(t, nil, a, Chunks(nil, a))
}

func TestDiffChunksDisjoint(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"x", "y", "z"}
	got := Chunks(a, b)
	want := []Chunk{{Change, 0, 2, 0, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestMyersFallbackLargeDistance(t *testing.T) {
	// two files with no common lines beyond the cap: must not blow up,
	// middle becomes one change Chunk
	var a, b []string
	for i := 0; i < 3000; i++ {
		a = append(a, "a"+string(rune('0'+i%10))+itoa(i))
		b = append(b, "b"+string(rune('0'+i%10))+itoa(i))
	}
	got := Chunks(a, b)
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

// Applying every change Chunk left→right must turn right into left.
func TestApplyAllMakesEqual(t *testing.T) {
	a := []string{"a", "b", "c", "d", "e", "f"}
	b := []string{"a", "x", "c", "e", "y", "f", "g"}
	right := append([]string{}, b...)
	for {
		chunks := Chunks(a, right)
		var c *Chunk
		for i := range chunks {
			if chunks[i].Kind == Change {
				c = &chunks[i]
				break
			}
		}
		if c == nil {
			break
		}
		right = Splice(right, c.R0, c.R1, a[c.L0:c.L1])
	}
	if !reflect.DeepEqual(right, a) {
		t.Fatalf("got %v want %v", right, a)
	}
}

func TestSplice(t *testing.T) {
	d := []string{"a", "b", "c"}
	if got := Splice(d, 1, 2, []string{"x", "y"}); !reflect.DeepEqual(got, []string{"a", "x", "y", "c"}) {
		t.Fatalf("got %v", got)
	}
	if got := Splice(d, 0, 3, nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := Splice(d, 3, 3, []string{"z"}); !reflect.DeepEqual(got, []string{"a", "b", "c", "z"}) {
		t.Fatalf("got %v", got)
	}
}
