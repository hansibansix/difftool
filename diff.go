package main

// Line-based Myers diff producing chunks usable for side-by-side rendering
// and chunk-wise apply.

type chunkKind int

const (
	kindEqual chunkKind = iota
	kindChange
)

// chunk covers the half-open line ranges [l0,l1) in the left file and
// [r0,r1) in the right file. Chunks partition both files contiguously.
type chunk struct {
	kind           chunkKind
	l0, l1, r0, r1 int
}

func diffChunks(a, b []string) []chunk {
	matches := myers(a, b)
	var chunks []chunk
	la, lb := 0, 0
	flushChange := func(l1, r1 int) {
		if la < l1 || lb < r1 {
			chunks = append(chunks, chunk{kindChange, la, l1, lb, r1})
		}
	}
	for _, mt := range matches {
		flushChange(mt[0], mt[1])
		if n := len(chunks); n > 0 && chunks[n-1].kind == kindEqual && chunks[n-1].l1 == mt[0] {
			chunks[n-1].l1++
			chunks[n-1].r1++
		} else {
			chunks = append(chunks, chunk{kindEqual, mt[0], mt[0] + 1, mt[1], mt[1] + 1})
		}
		la, lb = mt[0]+1, mt[1]+1
	}
	flushChange(len(a), len(b))
	return chunks
}

// myers returns the matched line pairs (i, j) of an LCS of a and b,
// in ascending order.
func myers(a, b []string) [][2]int {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	mid := myersCore(a[pre:len(a)-suf], b[pre:len(b)-suf])
	matches := make([][2]int, 0, pre+suf+len(mid))
	for i := 0; i < pre; i++ {
		matches = append(matches, [2]int{i, i})
	}
	for _, mt := range mid {
		matches = append(matches, [2]int{mt[0] + pre, mt[1] + pre})
	}
	for i := 0; i < suf; i++ {
		matches = append(matches, [2]int{len(a) - suf + i, len(b) - suf + i})
	}
	return matches
}

// Beyond this edit distance the trace memory would explode; the middle part
// is then reported as one big change instead of a fine-grained diff.
const maxEditDistance = 2000

func myersCore(a, b []string) [][2]int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return nil
	}
	lim := n + m
	if lim > maxEditDistance {
		lim = maxEditDistance
	}
	off := lim
	v := make([]int, 2*lim+2)
	var trace [][]int
	found := false
	var dEnd int
	for d := 0; d <= lim; d++ {
		vc := make([]int, len(v))
		copy(vc, v)
		trace = append(trace, vc)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				found = true
				dEnd = d
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return nil
	}
	var matches [][2]int
	x, y := n, m
	for d := dEnd; x > 0 || y > 0; d-- {
		vv := trace[d]
		k := x - y
		var pk int
		if k == -d || (k != d && vv[off+k-1] < vv[off+k+1]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := vv[off+pk]
		py := px - pk
		for x > px && y > py {
			x--
			y--
			matches = append(matches, [2]int{x, y})
		}
		if d > 0 {
			x, y = px, py
		}
	}
	// reverse into ascending order
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}

// splice returns dst with [from,to) replaced by a copy of src.
func splice(dst []string, from, to int, src []string) []string {
	out := make([]string, 0, len(dst)-(to-from)+len(src))
	out = append(out, dst[:from]...)
	out = append(out, src...)
	out = append(out, dst[to:]...)
	return out
}

// span is a half-open rune index range [a,b).
type span struct{ a, b int }

// changedSpans returns the rune ranges of a and b not covered by an LCS —
// the intraline difference between two paired lines.
func changedSpans(a, b []rune) ([]span, []span) {
	as := make([]string, len(a))
	for i, r := range a {
		as[i] = string(r)
	}
	bs := make([]string, len(b))
	for i, r := range b {
		bs[i] = string(r)
	}
	matches := myers(as, bs)
	return complementSpans(matches, 0, len(a)), complementSpans(matches, 1, len(b))
}

func complementSpans(matches [][2]int, side, n int) []span {
	var out []span
	prev := 0
	for _, m := range matches {
		p := m[side]
		if p > prev {
			out = append(out, span{prev, p})
		}
		prev = p + 1
	}
	if n > prev {
		out = append(out, span{prev, n})
	}
	return out
}
