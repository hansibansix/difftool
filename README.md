# difftool

Side-by-side terminal diff viewer with chunk-wise apply, in the spirit of
PhpStorm's diff window. Compares two files or two directories.

```
difftool [-theme name] <left> <right>   # two files or two directories
difftool [-theme name] -git [ref] [path]  # working tree vs. git ref (default HEAD)
```

In git mode the left side is the ref version (read-only); applying a chunk
or copying a file left → right reverts it in the working tree. Untracked
files are listed as "only right". An optional path (file or directory)
limits the comparison; a single existing path is taken as the path, not a
ref — use the two-arg form to disambiguate.

## File view

| Key           | Action                              |
|---------------|-------------------------------------|
| `n` / `p`     | next / previous change              |
| `l` `→` `>`   | apply current chunk left → right    |
| `h` `←` `<`   | apply current chunk right → left    |
| `a`           | apply ALL pending (then `l`/`h`)    |
| `x` / `X`     | reset applied hunk / all            |
| `u`           | undo last apply / reset / all       |
| `/` `n`/`N`   | search, next/prev match             |
| `J` / `K`     | next / prev file (dir mode)         |
| `s`           | save modified file(s)               |
| `j` / `k`     | scroll, `ctrl+d`/`ctrl+u` half page |
| `H` / `L`     | horizontal scroll (long lines)      |
| `i`           | toggle intraline highlight          |
| `,`           | settings menu                       |
| `g` / `G`     | top / bottom                        |
| `q` / `esc`   | quit (asks once on unsaved changes) |

## Directory view

Split layout: the file tree on the left, the diff of the selected file on
the right (the diff follows the selection; below 90 columns the panes
alternate full-screen instead). `.git` is skipped.

| Key         | Action                                   |
|-------------|------------------------------------------|
| `enter`/`tab` | focus the diff pane (`tab`/`q` returns) |
| `t`         | show / hide the tree pane (persisted)    |
| `/`         | filter the list (esc clears)             |
| `l` / `h`   | copy selected file to the other side     |
| `u`         | undo the last copy                       |
| `a`         | toggle showing identical files           |
| `j` / `k`   | move selection                           |
| `q` / `esc` | quit                                     |

Changed words within modified lines are emphasized (intraline diff).
A scrollbar strip on the right edge marks where changes and applied hunks
live in the file; line numbers are tinted by change type. Code is
syntax-highlighted (chroma, style matched to the theme; toggleable).
Mouse wheel scrolls (horizontal wheel too). `*` in the header marks unsaved changes.
Applied chunks stay tinted with a `▶`/`◀` arrow showing the copy direction,
remain reachable with `n`/`p`, and the view does not jump on apply.
Files made equal during the session stay listed as `✓ applied` in the
directory view.
The directory list groups files by folder with colored status glyphs.

## Settings

`?` shows a help overlay with all keys.
`,` opens the settings menu in any view: theme, syntax + intraline highlight,
ignore whitespace, tab width, show identical files, ignore patterns on/off.
Changes apply live and are saved to `~/.config/difftool/config.json` on close.

## Ignore patterns

Directory scans (and git mode) skip files and folders matching the glob
patterns in `ignore_patterns` (defaults: `node_modules`, `vendor`, VCS/IDE
metadata, `__pycache__`, minified assets, swap/OS junk); matching folders
are pruned entirely. Patterns without `/` match the basename at any depth,
patterns with `/` match the relative path. `-x 'pat,pat'` adds patterns for
one run; the settings menu toggles all patterns on/off.

## Themes

`rose-pine` (default), `catppuccin` (mocha), `nord`, `dracula`, `darcula`
(JetBrains), `gruvbox`, `tokyonight` — all truecolor — and `ansi` as
256-color fallback. Select in the settings menu, via `-theme <name>`, or
`DIFFTOOL_THEME=<name>` (precedence: flag > env > config).

## Build

```
go build -o ~/.local/bin/difftool .
```
