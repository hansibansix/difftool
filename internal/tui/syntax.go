package tui

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// fgSpan colors the rune range [a,b) of a line with fg.
type fgSpan struct {
	a, b int
	fg   string
}

// beyond this size highlighting is skipped so the diff stays fast
const maxHighlightBytes = 2 << 20

// highlightMsg delivers the spans computed for one side of m; gen tells a
// result whose lines changed meanwhile apart from the current one.
type highlightMsg struct {
	m    *model
	side int // 0 left, 1 right
	gen  int
	fgs  [][]fgSpan
}

// highlightCmd starts highlighting every side whose lines differ from what
// its spans were last requested for. Tokenizing runs off the update loop
// because it dominates the cost of opening a big file: the diff shows at
// once and colors in when the lexer is done. Until then the previous spans
// stay (bounds-checked by the renderers), which beats a flash of plain text
// on every apply.
func (m *model) highlightCmd() tea.Cmd {
	var cmds []tea.Cmd
	for side, lines := range [2][]string{m.left, m.right} {
		if sameLines(m.hlFor[side], lines) {
			continue
		}
		m.hlFor[side] = lines
		m.hlGen[side]++
		gen, path := m.hlGen[side], m.leftPath
		if side == 1 {
			path = m.rightPath
		}
		style := ""
		if cfg.Syntax {
			style = th.chromaStyle
		}
		expanded := expandAll(lines) // reads cfg, so not in the goroutine
		cmds = append(cmds, func() tea.Msg {
			return highlightMsg{m, side, gen, highlightLines(path, expanded, style)}
		})
	}
	return tea.Batch(cmds...)
}

func (m *model) setHighlight(msg highlightMsg) {
	if msg.gen != m.hlGen[msg.side] {
		return
	}
	if msg.side == 0 {
		m.leftFgs = msg.fgs
	} else {
		m.rightFgs = msg.fgs
	}
}

// highlightLines tokenizes lines (already tab-expanded, so rune columns
// match the display) and returns per-line foreground spans using the
// chroma style. Nil when the style is empty or highlighting is not possible.
func highlightLines(path string, lines []string, style string) [][]fgSpan {
	if style == "" {
		return nil
	}
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		return nil
	}
	size := 0
	for _, l := range lines {
		size += len(l) + 1
	}
	if size > maxHighlightBytes {
		return nil
	}
	st := styles.Get(style)
	it, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return nil
	}
	out := make([][]fgSpan, len(lines))
	line, col := 0, 0
	for _, tok := range it.Tokens() {
		fg := ""
		if e := st.Get(tok.Type); e.Colour.IsSet() {
			fg = e.Colour.String()
		}
		for j, part := range strings.Split(tok.Value, "\n") {
			if j > 0 {
				line++
				col = 0
			}
			if line >= len(out) {
				return out
			}
			n := utf8.RuneCountInString(part)
			if n > 0 && fg != "" {
				out[line] = append(out[line], fgSpan{col, col + n, fg})
			}
			col += n
		}
	}
	return out
}
