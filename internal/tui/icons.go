package tui

import (
	"path/filepath"
	"strings"
)

// Nerd Font glyphs (v3 codepoints in the BMP private-use area, so every
// terminal renders them one cell wide) with the usual devicon colors, for
// the tree pane. Keyed by lower-case extension, or by full file name for
// the well-known extensionless files. Written as \u escapes: editors and
// tools tend to drop raw private-use characters.
type icon struct{ glyph, color string }

var (
	iconPHP    = icon{"", "#a074c4"}
	iconJS     = icon{"", "#cbcb41"}
	iconTS     = icon{"", "#519aba"}
	iconJSON   = icon{"", "#cbcb41"}
	iconMD     = icon{"", "#519aba"}
	iconYAML   = icon{"", "#6d8086"}
	iconConf   = icon{"", "#6d8086"}
	iconShell  = icon{"", "#4d5a5e"}
	iconImage  = icon{"", "#a074c4"}
	iconZip    = icon{"", "#eca517"}
	iconGit    = icon{"", "#f14e32"}
	iconText   = icon{"", "#89e051"}
	iconC      = icon{"", "#599eff"}
	iconFolder = icon{"", "#7aa2f7"}
	iconFile   = icon{"", "#6d8086"}
)

var iconByExt = map[string]icon{
	"php": iconPHP, "js": iconJS, "mjs": iconJS, "ts": iconTS, "vue": {"", "#8dc149"},
	"json": iconJSON, "md": iconMD, "txt": iconText, "csv": {"", "#89e051"}, "pdf": {"", "#b30b00"},
	"css": {"", "#42a5f5"}, "scss": {"", "#f55385"}, "less": {"", "#563d7c"}, "html": {"", "#e44d26"},
	"mustache": {"", "#e37933"}, "xml": {"", "#e37933"},
	"yml": iconYAML, "yaml": iconYAML, "toml": {"", "#9c4221"}, "ini": iconConf, "conf": iconConf, "cfg": iconConf,
	"py": {"", "#ffbc03"}, "go": {"", "#519aba"}, "lua": {"", "#51a0cf"}, "rs": {"", "#dea584"},
	"rb": {"", "#701516"}, "java": {"", "#cc3e44"}, "c": iconC, "h": iconC, "cpp": {"", "#519aba"},
	"sh": iconShell, "bash": iconShell, "zsh": iconShell, "fish": iconShell,
	"sql": {"", "#dad8d8"}, "lock": {"", "#bbbbbb"},
	"png": iconImage, "jpg": iconImage, "jpeg": iconImage, "gif": iconImage, "webp": iconImage, "svg": iconImage, "ico": iconImage,
	"zip": iconZip, "tar": iconZip, "gz": iconZip, "tgz": iconZip,
}

var iconByName = map[string]icon{
	"dockerfile": {"", "#458ee6"}, ".gitignore": iconGit, ".gitattributes": iconGit, ".gitmodules": iconGit,
	"makefile": {"", "#6d8086"}, "license": {"", "#cbcb41"}, "readme": iconMD,
}

func fileIcon(path string) icon {
	name := strings.ToLower(filepath.Base(path))
	if ic, ok := iconByName[name]; ok {
		return ic
	}
	if ic, ok := iconByExt[strings.TrimPrefix(filepath.Ext(name), ".")]; ok {
		return ic
	}
	return iconFile
}
