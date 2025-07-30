package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var (
	helpFlag     bool
	versionFlag  bool
	verboseFlag  bool
	relativeFlag bool
	errorFlag    bool
	dryRunFlag   bool
)

const (
	version = "1.0.0"
)

func init() {
	flag.BoolVar(&helpFlag, "h", false, "Show help")
	flag.BoolVar(&versionFlag, "V", false, "Show version")
	flag.BoolVar(&verboseFlag, "v", false, "Enable verbose output")
	flag.BoolVar(&errorFlag, "e", false, "Exit on first error")
	flag.BoolVar(&relativeFlag, "r", false, "Use relative paths for symlinks")
	flag.BoolVar(&dryRunFlag, "n", false, "Dry run mode (do not modify links)")
	flag.Usage = func() {
		fmt.Println("Usage: rellink [options] LINKS...")
		flag.PrintDefaults()
	}
}

func isSymLink(s string) bool {
	i, err := os.Lstat(s)
	return err == nil && i.Mode()&os.ModeSymlink != 0
}

// realPath returns the absolute path of a symbolic link and resolves all parent symlinks.
func realPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlink: %w", err)
	}
	return resolvedPath, nil
}

func rellink(link string) error {
	if !isSymLink(link) {
		fmt.Fprintln(os.Stderr, "Error: Not a symbolic link:", link)
		return nil
	}
	oldTarget, err := os.Readlink(link)
	if err != nil {
		return fmt.Errorf("failed to read symlink %s: %w", link, err)
	}

	newTarget, err := realPath(link)
	if err != nil {
		return fmt.Errorf("failed to resolve link %s: %w", link, err)
	}

	if relativeFlag {
		// Create a relative symlink
		relPath, err := filepath.Rel(filepath.Dir(link), newTarget)
		if err != nil {
			return fmt.Errorf("failed to create relative path: %w", err)
		}
		newTarget = relPath
	}

	if oldTarget == newTarget {
		return nil
	}

	if verboseFlag || dryRunFlag {
		fmt.Printf("Updating link %s: %s -> %s\n", link, oldTarget, newTarget)
	}
	if dryRunFlag {
		return nil
	}

	if err = os.Remove(link); err != nil {
		return fmt.Errorf("failed to remove old symlink: %w", err)
	}

	// Create the new symlink
	if err = os.Symlink(newTarget, link); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}
	return nil
}

func main() {
	flag.Parse()

	if helpFlag {
		flag.Usage()
		return
	}

	if versionFlag {
		fmt.Println("Version", version)
		return
	}

	links := flag.Args()
	if len(links) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No links provided")
		flag.Usage()
		return
	}

	for _, link := range links {
		if err := rellink(link); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to process link:", link, "-", err)
			if errorFlag {
				os.Exit(1)
			}
		}
	}
}
