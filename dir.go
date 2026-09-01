package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type dirStatus int

const (
	stSame dirStatus = iota
	stModified
	stOnlyLeft
	stOnlyRight
	stApplied // was different, made equal in this session
)

func (s dirStatus) label() string {
	switch s {
	case stModified:
		return "modified"
	case stOnlyLeft:
		return "only left"
	case stOnlyRight:
		return "only right"
	case stApplied:
		return "applied"
	}
	return "same"
}

// glyph arrows point toward the side the file exists on
func (s dirStatus) glyph() string {
	switch s {
	case stModified:
		return "●"
	case stOnlyLeft:
		return "◂"
	case stOnlyRight:
		return "▸"
	case stApplied:
		return "✓"
	}
	return "·"
}

func (s dirStatus) style() lipgloss.Style {
	switch s {
	case stModified:
		return styleStModified
	case stOnlyLeft:
		return styleStOnlyLeft
	case stOnlyRight:
		return styleStOnlyRight
	case stApplied:
		return styleStApplied
	}
	return styleStSame
}

type dirEntry struct {
	rel    string
	status dirStatus
}

// dirRow is one visible list row: a directory group header or a file.
type dirRow struct {
	header string // non-empty = group header row
	n      int    // files in the group, for header rows
	ei     int    // index into entries for file rows
}

type dirModel struct {
	leftRoot, rightRoot string
	// display labels for the header; differ from the roots in git mode
	leftLabel, rightLabel string
	roLeft                bool // left side is a git ref: no copy toward it
	entries               []dirEntry
	rows                  []dirRow
	showAll               bool
	sel, top              int
	w, h                  int
	status                string
	filter                string
	filterInput           bool
}

func newDirModel(leftRoot, rightRoot string) (*dirModel, error) {
	d := &dirModel{leftRoot: leftRoot, rightRoot: rightRoot, showAll: cfg.ShowIdentical}
	if err := d.scan(); err != nil {
		return nil, err
	}
	if d.selected() == nil {
		d.status = "directories are identical"
	}
	return d, nil
}

func (d *dirModel) scan() error {
	seen := map[string]int{} // bit 1 = left, bit 2 = right
	walk := func(root string, bit int) error {
		return filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if de.IsDir() {
				if de.Name() == ".git" || (rel != "." && ignored(rel)) {
					return filepath.SkipDir
				}
				return nil
			}
			if !de.Type().IsRegular() || ignored(rel) {
				return nil
			}
			seen[rel] |= bit
			return nil
		})
	}
	if err := walk(d.leftRoot, 1); err != nil {
		return err
	}
	if err := walk(d.rightRoot, 2); err != nil {
		return err
	}
	rels := make([]string, 0, len(seen))
	for rel := range seen {
		rels = append(rels, rel)
	}
	// group files of a directory together
	sort.Slice(rels, func(i, j int) bool {
		di, dj := filepath.Dir(rels[i]), filepath.Dir(rels[j])
		if di != dj {
			return di < dj
		}
		return rels[i] < rels[j]
	})
	d.entries = d.entries[:0]
	for _, rel := range rels {
		d.entries = append(d.entries, dirEntry{rel, d.compare(rel, seen[rel])})
	}
	d.rebuildList()
	return nil
}

func (d *dirModel) compare(rel string, bits int) dirStatus {
	switch bits {
	case 1:
		return stOnlyLeft
	case 2:
		return stOnlyRight
	}
	if filesEqual(filepath.Join(d.leftRoot, rel), filepath.Join(d.rightRoot, rel)) {
		return stSame
	}
	return stModified
}

func filesEqual(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false
	}
	if ia.Size() != ib.Size() {
		return false
	}
	da, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false
	}
	return bytes.Equal(da, db)
}

func (d *dirModel) rebuildList() {
	prevEi := -1
	if r := d.rowAt(d.sel); r != nil && r.header == "" {
		prevEi = r.ei
	}
	prevSel := d.sel
	d.rows = d.rows[:0]
	lastDir := "\x00"
	lastHeader := -1
	for i, e := range d.entries {
		if !d.showAll && e.status == stSame {
			continue
		}
		if ignored(e.rel) {
			continue
		}
		if d.filter != "" && !strings.Contains(strings.ToLower(e.rel), strings.ToLower(d.filter)) {
			continue
		}
		if dir := filepath.Dir(e.rel); dir != lastDir {
			d.rows = append(d.rows, dirRow{header: dir + "/", ei: -1})
			lastDir = dir
			lastHeader = len(d.rows) - 1
		}
		d.rows = append(d.rows, dirRow{ei: i})
		d.rows[lastHeader].n++
	}
	d.sel = -1
	if prevEi >= 0 {
		for i, r := range d.rows {
			if r.header == "" && r.ei == prevEi {
				d.sel = i
				break
			}
		}
	}
	if d.sel < 0 {
		d.sel = d.snapToFile(clamp(prevSel, 0, max(0, len(d.rows)-1)))
	}
	d.ensureVisible()
}

func (d *dirModel) rowAt(i int) *dirRow {
	if i < 0 || i >= len(d.rows) {
		return nil
	}
	return &d.rows[i]
}

// snapToFile returns the file row nearest to i (forward first), or -1.
func (d *dirModel) snapToFile(i int) int {
	for j := i; j < len(d.rows); j++ {
		if d.rows[j].header == "" {
			return j
		}
	}
	for j := min(i, len(d.rows)-1); j >= 0; j-- {
		if d.rows[j].header == "" {
			return j
		}
	}
	return -1
}

func (d *dirModel) selected() *dirEntry {
	if r := d.rowAt(d.sel); r != nil && r.header == "" {
		return &d.entries[r.ei]
	}
	return nil
}

// refreshSelected re-compares the selected pair, e.g. after the file diff
// view saved one side.
func (d *dirModel) refreshSelected() {
	e := d.selected()
	if e == nil {
		return
	}
	bits := 0
	if _, err := os.Stat(filepath.Join(d.leftRoot, e.rel)); err == nil {
		bits |= 1
	}
	if _, err := os.Stat(filepath.Join(d.rightRoot, e.rel)); err == nil {
		bits |= 2
	}
	old := e.status
	e.status = d.compare(e.rel, bits)
	// a pair synced in this session stays visible instead of vanishing
	if e.status == stSame && old != stSame {
		e.status = stApplied
	}
	d.rebuildList()
}

func (d *dirModel) bodyH() int { return max(1, d.h-2) }

func (d *dirModel) ensureVisible() {
	if d.sel < 0 {
		return
	}
	if d.sel < d.top {
		d.top = d.sel
		// pull the group header into view when its first file is selected
		if d.sel > 0 && d.rows[d.sel-1].header != "" {
			d.top = d.sel - 1
		}
	}
	if d.sel >= d.top+d.bodyH() {
		d.top = d.sel - d.bodyH() + 1
	}
	d.top = clamp(d.top, 0, max(0, len(d.rows)-d.bodyH()))
}

func (d *dirModel) move(delta int) {
	if d.sel < 0 {
		return
	}
	step := 1
	if delta < 0 {
		step, delta = -1, -delta
	}
	for n := 0; n < delta; n++ {
		j := d.sel + step
		for j >= 0 && j < len(d.rows) && d.rows[j].header != "" {
			j += step
		}
		if j < 0 || j >= len(d.rows) {
			break
		}
		d.sel = j
	}
	d.ensureVisible()
}

func (d *dirModel) copyEntry(toRight bool) {
	e := d.selected()
	if e == nil {
		return
	}
	src := filepath.Join(d.leftRoot, e.rel)
	dst := filepath.Join(d.rightRoot, e.rel)
	arrow := "▶"
	if !toRight {
		if d.roLeft {
			d.status = "left side is read-only (git ref)"
			return
		}
		src, dst = dst, src
		arrow = "◀"
	}
	if _, err := os.Stat(src); err != nil {
		d.status = "file missing on source side — nothing to copy " + arrow
		return
	}
	if err := copyFile(src, dst); err != nil {
		d.status = "error: " + err.Error()
		return
	}
	e.status = stApplied
	d.status = "✓ copied " + arrow + " " + e.rel
	d.rebuildList()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

func (d *dirModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			d.move(-1)
		case tea.MouseButtonWheelDown:
			d.move(1)
		}
	case tea.KeyMsg:
		if d.filterInput {
			switch msg.String() {
			case "enter":
				d.filterInput = false
			case "esc", "ctrl+c":
				d.filterInput = false
				d.filter = ""
				d.rebuildList()
			case "backspace":
				if rs := []rune(d.filter); len(rs) > 0 {
					d.filter = string(rs[:len(rs)-1])
				}
				d.rebuildList()
			default:
				if msg.Type == tea.KeyRunes || msg.String() == " " {
					d.filter += string(msg.Runes)
					if msg.String() == " " {
						d.filter += " "
					}
					d.rebuildList()
				}
			}
			return nil
		}
		d.status = ""
		switch msg.String() {
		case "/":
			d.filterInput = true
			d.filter = ""
			d.rebuildList()
		case "q", "ctrl+c":
			return tea.Quit
		case "esc":
			if d.filter != "" {
				d.filter = ""
				d.rebuildList()
				return nil
			}
			return tea.Quit
		case "j", "down":
			d.move(1)
		case "k", "up":
			d.move(-1)
		case "ctrl+d":
			d.move(d.bodyH() / 2)
		case "ctrl+u":
			d.move(-d.bodyH() / 2)
		case "g":
			d.move(-len(d.rows))
		case "G":
			d.move(len(d.rows))
		case "a":
			d.showAll = !d.showAll
			cfg.ShowIdentical = d.showAll
			d.rebuildList()
		case "l", "right", ">":
			d.copyEntry(true)
		case "h", "left", "<":
			d.copyEntry(false)
		}
	}
	return nil
}

func (d *dirModel) view() string {
	if d.w == 0 || d.h == 0 {
		return ""
	}
	var b strings.Builder
	tail := func(p string, w int) string {
		if runewidth.StringWidth(p) > w {
			return runewidth.TruncateLeft(p, runewidth.StringWidth(p)-w+1, "…")
		}
		return p
	}
	lh, rh := d.leftRoot, d.rightRoot
	if d.leftLabel != "" {
		lh = d.leftLabel
	}
	if d.rightLabel != "" {
		rh = d.rightLabel
	}
	sideW := max(4, (d.w-5)/2)
	head := styleBar.Render(" ") + styleHeaderText.Render(tail(lh, sideW)) +
		styleHeaderDim.Render(" ⇄ ") + styleHeaderText.Render(tail(rh, sideW))
	b.WriteString(barPad(head, d.w) + "\n")

	for i := d.top; i < d.top+d.bodyH(); i++ {
		if i >= len(d.rows) {
			b.WriteString(styleGutter.Render("~") + "\n")
			continue
		}
		r := d.rows[i]
		if r.header != "" {
			b.WriteString("  " + styleGroup.Render(tail(r.header, max(4, d.w-10))) +
				styleStSame.Render(fmt.Sprintf(" · %d", r.n)) + "\n")
			continue
		}
		e := d.entries[r.ei]
		label := e.status.label()
		glyphSt, labelSt, nameSt := e.status.style(), e.status.style(), lipgloss.NewStyle()
		mark, pad := " ", lipgloss.NewStyle()
		if i == d.sel {
			bg := lipgloss.Color(th.selBg)
			glyphSt, labelSt = glyphSt.Background(bg), labelSt.Background(bg)
			nameSt, pad = styleSelected, styleSelected
			mark = styleMark.Render("▌")
		}
		nameW := max(4, d.w-4-2-runewidth.StringWidth(label)-3)
		name := runewidth.Truncate(filepath.Base(e.rel), nameW, "…")
		gap := strings.Repeat(" ", max(1, nameW-runewidth.StringWidth(name)+1))
		b.WriteString(mark + pad.Render("   ") + glyphSt.Render(e.status.glyph()) +
			nameSt.Render(" "+name+gap) + labelSt.Render(label) + pad.Render(" ") + "\n")
	}

	info := d.countsInfo()
	if d.filter != "" && !d.filterInput {
		info += " · /" + d.filter
	}
	status := d.status
	if d.filterInput {
		status = "/" + d.filter + "▏"
	}
	b.WriteString(footerBar(d.w, status, info, [][2]string{
		{"enter", "open"}, {"h·l", "◀ copy ▶"}, {"/", "filter"},
		{"a", "show all"}, {"?", "help"}, {"q", "quit"},
	}))
	return b.String()
}

func (d *dirModel) countsInfo() string {
	var nm, nl, nr, na int
	for _, e := range d.entries {
		switch e.status {
		case stModified:
			nm++
		case stOnlyLeft:
			nl++
		case stOnlyRight:
			nr++
		case stApplied:
			na++
		}
	}
	var parts []string
	if nm > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", nm))
	}
	if nl > 0 {
		parts = append(parts, fmt.Sprintf("%d only left", nl))
	}
	if nr > 0 {
		parts = append(parts, fmt.Sprintf("%d only right", nr))
	}
	if na > 0 {
		parts = append(parts, fmt.Sprintf("%d applied", na))
	}
	if len(parts) == 0 {
		return "no differences"
	}
	return strings.Join(parts, " · ")
}

func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}
