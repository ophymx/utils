package txtedit

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
)

// Result contains information about an edit session.
type Result struct {
	// Content is the final file content after editing.
	Content []byte
	// Path is the temporary file path used for editing when KeepTempFile is used.
	// Otherwise Path is empty.
	Path string
	// Changed indicates whether the edited content differs from the input.
	Changed bool
}

// Config configures how Edit runs.
type Config struct {
	Pattern       string
	TempDir       string
	EditorCommand []string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	KeepTempFile  bool
}

// DefaultConfig returns the default configuration for an edit session.
func DefaultConfig() Config {
	return Config{
		Pattern: "*.txt",
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
}

func applyConfigDefaults(cfg Config) Config {
	if cfg.Pattern == "" {
		cfg.Pattern = "*.txt"
	}
	if cfg.Stdin == nil {
		cfg.Stdin = os.Stdin
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	return cfg
}

// ResolveEditorCommand returns the editor command to run.
//
// Resolution order:
// 1. VISUAL
// 2. EDITOR
// 3. Platform defaults via ResolveDefaultEditorCommand
//
// Environment values are split on whitespace and are best-effort for
// simple command lines. For complex command lines (quoted paths or
// shell-specific escaping), prefer Config.EditorCommand.
func ResolveEditorCommand() ([]string, error) {
	for _, envName := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			parts := strings.Fields(value)
			if len(parts) == 0 {
				continue
			}
			if _, err := exec.LookPath(parts[0]); err != nil {
				return nil, fmt.Errorf("%s command %q not found: %w", envName, parts[0], err)
			}
			return parts, nil
		}
	}

	return ResolveDefaultEditorCommand()
}

// ResolveDefaultEditorCommand returns a reasonable default editor command
// based on the current platform.
func ResolveDefaultEditorCommand() ([]string, error) {
	return resolveDefaultEditorCommandForGOOS(runtime.GOOS)
}

func resolveDefaultEditorCommandForGOOS(goos string) ([]string, error) {
	for _, candidate := range defaultEditorCandidates(goos) {
		if len(candidate) == 0 || candidate[0] == "" {
			continue
		}
		if _, err := exec.LookPath(candidate[0]); err == nil {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("no default editor found for %s; set VISUAL/EDITOR or set Config.EditorCommand", goos)
}

func defaultEditorCandidates(goos string) [][]string {
	var candidates [][]string
	switch goos {
	case "windows":
		candidates = [][]string{{"notepad.exe"}, {"notepad"}, {"nvim"}, {"vim"}, {"nano"}}
	case "darwin":
		candidates = [][]string{{"vi"}, {"vim"}, {"nvim"}, {"nano"}, {"open", "-W", "-n", "-a", "TextEdit"}}
	default:
		candidates = [][]string{{"editor"}, {"sensible-editor"}, {"vi"}, {"vim"}, {"nvim"}, {"nano"}}
	}

	return candidates
}

// Edit opens a temporary file in an editor and returns the edited content.
func Edit(initial []byte, cfg Config) (Result, error) {
	cfg = applyConfigDefaults(cfg)

	tmpFile, err := os.CreateTemp(cfg.TempDir, cfg.Pattern)
	if err != nil {
		return Result{}, fmt.Errorf("create temp file: %w", err)
	}

	path := tmpFile.Name()
	if !cfg.KeepTempFile {
		defer os.Remove(path)
	}

	if _, err := tmpFile.Write(initial); err != nil {
		tmpFile.Close()
		return Result{}, fmt.Errorf("write initial content: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return Result{}, fmt.Errorf("close temp file before edit: %w", err)
	}

	command := cfg.EditorCommand
	if len(command) == 0 {
		if command, err = ResolveEditorCommand(); err != nil {
			return Result{}, err
		}
	}

	if _, err := exec.LookPath(command[0]); err != nil {
		return Result{}, fmt.Errorf("editor %q not found: %w", command[0], err)
	}

	args := append(slices.Clone(command[1:]), path)
	cmd := exec.Command(command[0], args...)
	cmd.Stdin = cfg.Stdin
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("run editor: %w", err)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read edited file: %w", err)
	}

	resultPath := ""
	if cfg.KeepTempFile {
		resultPath = path
	}

	return Result{
		Content: edited,
		Path:    resultPath,
		Changed: !bytes.Equal(initial, edited),
	}, nil
}

// EditString is a string helper around Edit.
func EditString(initial string, cfg Config) (string, error) {
	result, err := Edit([]byte(initial), cfg)
	if err != nil {
		return "", err
	}
	return string(result.Content), nil
}
