# txtedit/v2

`txtedit/v2` provides a small API for editing text by opening a temporary file in an external editor.

This is inspired by tools like `git` that create temporary files
for interactive rebase, or commit messages in order to drive actions.

**NOTE**: Improved from `v1` using __AI__ assistance.

## Install

This package is part of:

- `github.com/ophymx/utils`

Import path:

- `github.com/ophymx/utils/txtedit/v2`

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/ophymx/utils/txtedit/v2"
)

func main() {
    cfg := txtedit.DefaultConfig()
    cfg.Pattern = "note-*.txt"

    edited, err := txtedit.EditString("hello\n", cfg)
    if err != nil {
        panic(err)
    }
    fmt.Println(edited)
}
```

## API

### Core functions

- `DefaultConfig() Config`
- `Edit(initial []byte, cfg Config) (Result, error)`
- `EditString(initial string, cfg Config) (string, error)`

### Config

`Config` is the primary way to configure editing behavior.

- `Pattern string`: temp file pattern (defaults to `*.txt`)
- `TempDir string`: temp directory for `os.CreateTemp`
- `EditorCommand []string`: explicit argv command for the editor
- `Stdin io.Reader`, `Stdout io.Writer`, `Stderr io.Writer`: editor process IO
- `KeepTempFile bool`: when true, temp file is preserved after editing

### Result

- `Content []byte`: edited content
- `Path string`: path to the temporary file when `KeepTempFile` is used; otherwise empty
- `Changed bool`: whether content changed from input

### Editor resolution

When no editor override is provided, `ResolveEditorCommand` uses:

1. `VISUAL`
2. `EDITOR`
3. Platform defaults via `ResolveDefaultEditorCommand()`

Default candidates are platform-specific:

- Windows: `notepad.exe`, `notepad`, then common terminal editors
- macOS: `vi`, `vim`, `nvim`, `nano`, then `open -W -n -a TextEdit`
- Other Unix-like systems: `editor`, `sensible-editor`, `vi`, `vim`, `nvim`, `nano`

Environment values are split on whitespace and work best for simple command lines.
For complex command lines (quoted paths or shell-specific escaping), prefer
`Config.EditorCommand`.

### Windows Notes

**NOTE**: I did not test this thuroughly on a Windows system, PRs welcomed.

Windows does not define a universal system-level editor variable. In practice,
many tools still honor `VISUAL` and `EDITOR`, and this package does too.

Typical values:

- `notepad.exe`
- `code --wait`
- `nvim-qt --nofork`

If your editor path contains spaces or requires complex quoting, use
`Config.EditorCommand` instead of relying on environment parsing.

PowerShell examples:

```powershell
$env:EDITOR = "notepad.exe"
$env:VISUAL = "code --wait"
```

cmd.exe examples:

```bat
set EDITOR=notepad.exe
set VISUAL=code --wait
```

## Notes

- This package is intentionally not context-aware because the primary use case is interactive editing.
- By default, temporary files are deleted after editing. Set `Config.KeepTempFile = true` when you need to inspect the intermediate file.

## Migration from v1

v1 call:

```go
edited, err := txtedit.EditTempFile(contents, "mvit-*.txt")
```

v2 equivalent:

```go
cfg := txtedit.DefaultConfig()
cfg.Pattern = "mvit-*.txt"
edited, err := txtedit.EditString(contents, cfg)
```
