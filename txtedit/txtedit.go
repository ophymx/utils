package txtedit

import (
	"fmt"
	"os"
	"os/exec"
)

// DefaultEditor is the default text editor used if no other is specified.
var DefaultEditor = "vi"

// GetEditor returns the editor to be used, either from the EDITOR environment variable or the default editor.
// It also checks if the editor is available in the system's PATH.
func GetEditor() (editor string, err error) {
	if editor = os.Getenv("EDITOR"); editor == "" {
		editor = DefaultEditor
	}
	if _, err = exec.LookPath(editor); err != nil {
		return "", fmt.Errorf("editor %s not found", editor)
	}
	return editor, nil
}

// EditTempFile creates a temporary file with the given contents and opens it in the editor for editing.
// The edited contents are returned as a string.
func EditTempFile(contents string, pattern string) (edited string, err error) {
	if pattern == "" {
		pattern = "*.txt"
	}
	var f *os.File
	if f, err = os.CreateTemp("", pattern); err != nil {
		return
	}
	var tmpFilename = f.Name()
	f.WriteString(contents)
	if err = f.Close(); err != nil {
		return
	}
	defer os.Remove(tmpFilename)

	var editor string
	if editor, err = GetEditor(); err != nil {
		return
	}
	cmd := exec.Command(editor, tmpFilename)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err = cmd.Run(); err != nil {
		return
	}

	var b []byte
	if b, err = os.ReadFile(tmpFilename); err != nil {
		return
	}

	edited = string(b)
	return
}
