# difftool

Side-by-side terminal diff viewer with chunk-wise apply, in the spirit of
PhpStorm's diff window. Compares two files or two directories.

```
difftool [-theme name] <left> <right>       # two files or two directories
difftool [-theme name] -git [ref] [path]    # working tree vs. git ref (default HEAD)
difftool [-theme name] -git A..B [path]     # two refs, both read-only
difftool -merge LOCAL BASE REMOTE MERGED    # 3-way merge (git mergetool)
```

In git mode the left side is the ref version (read-only); applying a chunk
or copying a file left → right reverts it in the working tree. Untracked
files are listed as "only right". An optional path (file or directory)
limits the comparison; a single existing path is taken as the path, not a
ref — use the two-arg form to disambiguate. `A..B` compares two refs
without touching the working tree.

## Merge mode

3-way merge for `git mergetool`: the right pane is the merge result (seeded
by `git merge-file`; conflicts appear as marker blocks, each diffing as one
hunk), the left pane shows LOCAL, BASE or REMOTE (`1`/`2`/`3`). Applying a
hunk onto a conflict block resolves it with that side; `s` writes MERGED.
The exit code is 1 while conflicts remain, so enable `trustExitCode`:

```
[merge]
    tool = difftool
[mergetool "difftool"]
    cmd = difftool -merge "$LOCAL" "$BASE" "$REMOTE" "$MERGED"
    trustExitCode = true
```

## File view

| Key           | Action                              |
|---------------|-------------------------------------|
| `n` / `p`     | next / previous change              |
| `l` `→` `>`   | apply current chunk left → right    |
| `h` `←` `<`   | apply current chunk right → left    |
| `a`           | apply ALL pending (then `l`/`h`)    |
| `v`           | select lines (`j`/`k`), `l`/`h` applies them |
| `x` / `X`     | reset applied hunk / all            |
| `u`           | undo last apply / reset / all       |
| `/` `n`/`N`   | search, next/prev match             |
| `J` / `K`     | next / prev file (dir mode)         |
| `s`           | save modified file(s)               |
| `e` / `E`     | edit the right / left file in `$VISUAL`/`$EDITOR` at the current hunk; the diff reloads on exit |
| `P`           | export pending hunks as a unified patch (clipboard via wl-copy/xclip/xsel/pbcopy, else `./difftool.patch`); in visual mode only the current hunk |
| `j` / `k`     | scroll, `ctrl+d`/`ctrl+u` half page |
| `H` / `L`     | horizontal scroll (long lines)      |
| `i`           | toggle intraline highlight          |
| `w`           | toggle line wrap (persisted)        |
| `o`           | toggle unified one-column view (persisted) |
| `z`           | fold unchanged lines to 3 of context; click a fold to expand it (persisted) |
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
| `l` / `h`   | copy selected file to the other side; for a one-sided file the other direction deletes it (asks `y`) |
| `A`         | sync ALL listed files (`l`/`h`, then `y`)  |
| `I`         | add an ignore pattern (prefilled with the file name) |
| `u`         | undo the last copy / delete / sync       |
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
The directory list groups files by folder with colored status glyphs and a
per-file diffstat (`+12 −3`). Mouse: click a file or hunk to select it,
click a pane to focus it, the wheel scrolls the pane under the pointer.

## Settings

`?` shows a help overlay with all keys.
`,` opens the settings menu in any view: theme, syntax + intraline highlight,
line wrap, unified view, folding, ignore whitespace, ignore blank-line
changes, ignore lines matching a regex, tab width, show identical files,
ignore patterns on/off.
Changes apply live and are saved to `~/.config/difftool/config.json` on close.

Ignore rules: with *ignore blank-line changes* a hunk made only of blank
lines is shown plain and skipped by `n`/`p`, apply-all and patch export;
*ignore lines matching* (`ignore_regex`, e.g. `^\s*//` or `\$Id:`) does the
same for hunks whose lines all match. The footer counts them as `N ignored`.
In git mode the patch uses repo-relative `a/` `b/` paths, so
`P` + `git apply --cached` stages exactly the shown hunks.

## Ignore patterns

Directory scans (and git mode) skip files and folders matching the glob
patterns in `ignore_patterns` (defaults: `node_modules`, `vendor`, VCS/IDE
metadata, `__pycache__`, minified assets, swap/OS junk); matching folders
are pruned entirely. Patterns without `/` match any path component (a
directory name hides its whole subtree), patterns with `/` match the
relative path. `-x 'pat,pat'` adds patterns for
one run; the settings menu toggles all patterns on/off, and `enter` on that
entry opens an editor to add (`a`) and delete (`d`) patterns.

## Key bindings

Every key in the tables above can be rebound in `config.json` under `keys`,
per context (`file`, `dir`, `global`) and action. Only the actions you list
change; the rest keep their defaults. `difftool -keys` prints the complete
current map as a snippet to start from, and the help overlay and footer
hints always show the active bindings.

```json
"keys": {
  "file": { "apply-right": ["l", "right"], "apply-left": ["h", "left"], "save": ["ctrl+s"] },
  "dir":  { "quit": ["q"] },
  "global": { "help": ["f1", "?"] }
}
```

Key names are bubbletea's: letters as typed, `ctrl+x`, `enter`, `esc`,
`tab`, `up`/`down`/`left`/`right`, `f1`…`f12`, `" "` for space (all lowercase). `ctrl+c` always quits
and `esc` always cancels an input. Unknown actions are reported on stderr at
startup.

## Themes

`rose-pine` (default), `catppuccin` (mocha), `nord`, `dracula`, `darcula`
(JetBrains), `gruvbox`, `tokyonight` — all truecolor — and `ansi` as
256-color fallback. Select in the settings menu, via `-theme <name>`, or
`DIFFTOOL_THEME=<name>` (precedence: flag > env > config).

## Install

### Prebuilt binary

Every `v*` tag publishes archives for linux and macOS on amd64 and arm64.
The `latest/download` URL always points at the newest release:

```sh
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -fsSL "https://github.com/hansibansix/difftool/releases/latest/download/difftool_${os}_${arch}.tar.gz" |
    tar -xz -C ~/.local/bin
```

Swap `latest/download` for `download/v0.1.1` to pin a version. To verify a
download, grab `checksums.txt` from the same release and run
`sha256sum --ignore-missing -c checksums.txt` beside the archive.

### go install

```sh
go install github.com/hansibansix/difftool@latest
```

Puts the binary in `$(go env GOPATH)/bin`.

### From source

```sh
git clone https://github.com/hansibansix/difftool.git
cd difftool
go build -o ~/.local/bin/difftool .
```

Requires the Go version in `go.mod`; there are no cgo dependencies, so
`GOOS`/`GOARCH` cross-compile out of the box.

Make sure the target directory is on your `PATH`.
