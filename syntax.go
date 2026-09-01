package main

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// fgSpan colors the rune range [a,b) of a line with fg.
type fgSpan struct {
	a, b int
	fg   string
}

// beyond this size highlighting is skipped so the diff stays fast
const maxHighlightBytes = 2 << 20

// highlightLines tokenizes lines (already tab-expanded, so rune columns
// match the display) and returns per-line foreground spans using the
// theme's chroma style. Nil when highlighting is off or not possible.
func highlightLines(path string, lines []string) [][]fgSpan {
	if !cfg.Syntax || th.chromaStyle == "" {
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
	style := styles.Get(th.chromaStyle)
	it, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return nil
	}
	out := make([][]fgSpan, len(lines))
	line, col := 0, 0
	for _, tok := range it.Tokens() {
		fg := ""
		if e := style.Get(tok.Type); e.Colour.IsSet() {
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
