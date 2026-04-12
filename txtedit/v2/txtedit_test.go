package txtedit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireSh(t *testing.T) string {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	return shPath
}

func TestEditChangedAndCleanup(t *testing.T) {
	shPath := requireSh(t)
	cleanupDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.TempDir = cleanupDir
	cfg.Pattern = "cleanup-*.txt"
	cfg.EditorCommand = []string{shPath, "-c", "printf '%s\\n' after > \"$1\"", "--"}

	result, err := Edit([]byte("before\n"), cfg)
	if err != nil {
		t.Fatalf("Edit returned error: %v", err)
	}

	if got := string(result.Content); got != "after\n" {
		t.Fatalf("unexpected edited content: %q", got)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	if result.Path != "" {
		t.Fatalf("expected empty Path when temp file is not kept, got: %q", result.Path)
	}
	leftovers, globErr := filepath.Glob(filepath.Join(cleanupDir, "cleanup-*.txt"))
	if globErr != nil {
		t.Fatalf("glob failed: %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("expected no leftover temp files, got: %#v", leftovers)
	}
}

func TestEditNoChange(t *testing.T) {
	shPath := requireSh(t)
	cfg := DefaultConfig()
	cfg.EditorCommand = []string{shPath, "-c", ":", "--"}

	result, err := Edit([]byte("same\n"), cfg)
	if err != nil {
		t.Fatalf("Edit returned error: %v", err)
	}

	if got := string(result.Content); got != "same\n" {
		t.Fatalf("unexpected edited content: %q", got)
	}
	if result.Changed {
		t.Fatalf("expected Changed=false")
	}
}

func TestKeepTempFile(t *testing.T) {
	shPath := requireSh(t)
	keepDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.TempDir = keepDir
	cfg.Pattern = "keep-*.txt"
	cfg.KeepTempFile = true
	cfg.EditorCommand = []string{shPath, "-c", ":", "--"}

	result, err := Edit([]byte("content\n"), cfg)
	if err != nil {
		t.Fatalf("Edit returned error: %v", err)
	}

	if !strings.HasPrefix(result.Path, keepDir+string(os.PathSeparator)) {
		t.Fatalf("expected temp path in keep dir, got: %s", result.Path)
	}
	if _, statErr := os.Stat(result.Path); statErr != nil {
		t.Fatalf("expected temp file to exist, stat error: %v", statErr)
	}
}

func TestResolveEditorCommandPrefersVisual(t *testing.T) {
	t.Setenv("VISUAL", "sh -c")
	t.Setenv("EDITOR", "false")

	cmd, err := ResolveEditorCommand()
	if err != nil {
		t.Fatalf("ResolveEditorCommand returned error: %v", err)
	}
	if len(cmd) < 2 || cmd[0] != "sh" || cmd[1] != "-c" {
		t.Fatalf("unexpected command resolution: %#v", cmd)
	}
}

func TestResolveEditorCommandInvalidVisual(t *testing.T) {
	t.Setenv("VISUAL", "definitely-not-a-real-editor-binary")
	t.Setenv("EDITOR", "")

	_, err := ResolveEditorCommand()
	if err == nil {
		t.Fatalf("expected error for invalid VISUAL")
	}
	if !strings.Contains(err.Error(), "VISUAL command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultEditorCandidatesByPlatform(t *testing.T) {
	windows := defaultEditorCandidates("windows")
	if len(windows) == 0 || len(windows[0]) == 0 || windows[0][0] != "notepad.exe" {
		t.Fatalf("unexpected windows default candidates: %#v", windows)
	}

	darwin := defaultEditorCandidates("darwin")
	if len(darwin) == 0 || len(darwin[0]) == 0 || darwin[0][0] != "vi" {
		t.Fatalf("unexpected darwin default candidates: %#v", darwin)
	}

	linux := defaultEditorCandidates("linux")
	if len(linux) == 0 || len(linux[0]) == 0 || linux[0][0] != "editor" {
		t.Fatalf("unexpected linux default candidates: %#v", linux)
	}
}

func TestEditWithExplicitConfig(t *testing.T) {
	shPath := requireSh(t)

	result, err := Edit([]byte("before\n"), Config{
		Pattern:       "cfg-*.txt",
		EditorCommand: []string{shPath, "-c", "printf '%s\\n' after > \"$1\"", "--"},
	})
	if err != nil {
		t.Fatalf("Edit returned error: %v", err)
	}

	if got := string(result.Content); got != "after\n" {
		t.Fatalf("unexpected edited content: %q", got)
	}
	if result.Path != "" {
		t.Fatalf("expected empty Path by default with direct Config, got: %q", result.Path)
	}
}

func TestConfigEditorCommandPreservesEmptyArgs(t *testing.T) {
	shPath := requireSh(t)
	cfg := DefaultConfig()
	cfg.EditorCommand = []string{shPath, "-c", `if [ "$1" = "" ]; then printf 'ok' > "$2"; else printf 'bad' > "$2"; fi`, "--", ""}

	result, err := Edit([]byte("before\n"), cfg)
	if err != nil {
		t.Fatalf("Edit returned error: %v", err)
	}

	if got := strings.TrimSpace(string(result.Content)); got != "ok" {
		t.Fatalf("expected empty arg to be preserved, got content: %q", got)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Pattern != "*.txt" {
		t.Fatalf("unexpected default pattern: %q", cfg.Pattern)
	}
	if cfg.Stdin == nil || cfg.Stdout == nil || cfg.Stderr == nil {
		t.Fatalf("expected default IO streams to be populated")
	}
}
