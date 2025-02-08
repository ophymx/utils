// Package main provides the mvit tool, which allows users to rename multiple files interactively.
// Users can edit a temporary file with the new names, keeping the index the same.
//
// The file format for the temporary file is as follows:
// Each line should contain an index and the new filename, separated by a colon.
// Lines starting with '#' are considered comments and are ignored.
// Example:
// 0: newname1.txt
// 1: newname2.txt
// # This is a comment
// 2: newname3.txt

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/ophymx/utils/txtedit"
)

// Flags for command-line options
var (
	helpFlag        bool
	versionFlag     bool
	verboseFlag     bool
	changeFlag      bool
	interactiveFlag bool
	noClobberFlag   bool
)

// Version of the mvit tool
const version = "0.1"

// Description of the mvit tool
const description = `mvit - rename multiple files interactively
       edit the temporary file with the new names,
       keeping the index the same`

func init() {
	// Initialize command-line flags
	flag.BoolVar(&helpFlag, "help", false, "Display help")
	flag.BoolVar(&helpFlag, "h", false, "Display help")
	flag.BoolVar(&versionFlag, "version", false, "Display version")
	flag.BoolVar(&versionFlag, "V", false, "Display version")
	flag.BoolVar(&verboseFlag, "v", true, "Verbose output")
	flag.BoolVar(&changeFlag, "c", false, "Only display changes")
	flag.BoolVar(&interactiveFlag, "i", true, "Interactive mode")
	flag.BoolVar(&noClobberFlag, "n", false, "No clobber mode")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] file1 file2 ...\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		fmt.Fprintf(flag.CommandLine.Output(), "%s\n", description)
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		flag.PrintDefaults()
	}
}

// mvitFmt returns the format string for the given total number of files.
func mvitFmt(total int) string {
	switch {
	case total < 10:
		return "%d: %s\n"
	case total < 100:
		return "%02d: %s\n"
	case total < 1000:
		return "%03d: %s\n"
	case total < 10000:
		return "%04d: %s\n"
	case total < 100000:
		return "%05d: %s\n"
	case total < 1000000:
		return "%06d: %s\n"
	default:
		panic("number of files way too large")
	}
}

// parseRenames parses the edited contents and returns a map of index to new filenames.
func parseRenames(maxIdx int, contents string) (map[int]string, error) {
	renames := make(map[int]string)
	for _, line := range strings.Split(strings.TrimSuffix(contents, "\n"), "\n") {
		if line == "" || strings.TrimLeft(line, " ")[0] == '#' {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New("invalid line: " + line)
		}
		var index int
		var err error
		if index, err = strconv.Atoi(parts[0]); err != nil {
			return nil, fmt.Errorf("invalid index: %s", parts[0])
		}
		if _, present := renames[index]; present {
			return nil, fmt.Errorf("%d is a duplicate", index)
		}
		renames[index] = strings.TrimSuffix(strings.TrimPrefix(parts[1], " "), "\n")
	}
	for index := range renames {
		if index > maxIdx {
			return nil, fmt.Errorf("%d is out of range", index)
		}
	}

	return renames, nil
}

// exists checks if a file exists.
func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// doRenames renames the files based on the provided map of index to new filenames.
func rename(files []string, renames map[int]string) error {
	for index, filename := range files {
		if update, present := renames[index]; present {
			if update == filename {
				if verboseFlag {
					fmt.Printf("`%s' unchanged\n", shellescape.Quote(filename))
				}
			} else {
				if changeFlag || verboseFlag {
					fmt.Printf("`%s' -> `%s'\n", shellescape.Quote(filename), shellescape.Quote(update))
				}
				if exists(update) {
					if noClobberFlag {
						fmt.Printf("`%s' already exists, skipping\n", shellescape.Quote(update))
						continue
					} else if interactiveFlag {
						var response string
						fmt.Printf("`%s' already exists, overwrite? [y/N] ", shellescape.Quote(update))
						fmt.Scanln(&response)
						if response != "y" && response != "Y" {
							continue
						}
					}
				}
				if err := os.Rename(filename, update); err != nil {
					return fmt.Errorf("error renaming `%s' to `%s': %w", shellescape.Quote(filename), shellescape.Quote(update), err)
				}
			}
		} else {
			if verboseFlag {
				fmt.Printf("`%s' unchanged\n", shellescape.Quote(filename))
			}
		}
	}

	return nil
}

// mvit renames the files based on the edited contents.
func mvit(files []string) (err error) {
	var sb strings.Builder
	format := mvitFmt(len(files))
	for index, filename := range files {
		sb.WriteString(fmt.Sprintf(format, index, filename))
	}

	var edited string
	edited, err = txtedit.EditTempFile(sb.String(), "mvit-*.txt")
	if err != nil {
		return fmt.Errorf("error editing file: %w", err)
	}
	renames, err := parseRenames(len(files)-1, edited)
	if err != nil {
		return fmt.Errorf("error parsing mvit tempfile: %w", err)
	}

	return rename(files, renames)
}

// dedupe removes duplicate filenames from the list.
func dedupe(filenames []string) []string {
	uniqueFilenames := make(map[string]bool)
	var dedupedFilenames []string
	for _, filename := range filenames {
		if !uniqueFilenames[filename] {
			uniqueFilenames[filename] = true
			dedupedFilenames = append(dedupedFilenames, filename)
		} else if verboseFlag {
			fmt.Printf("`%s' input is a duplicate, skipping\n", shellescape.Quote(filename))
		}
	}
	return dedupedFilenames
}

func main() {
	flag.Parse()
	if helpFlag {
		flag.Usage()
		os.Exit(0)
	}
	if versionFlag {
		fmt.Println(version)
		os.Exit(0)
	}
	if changeFlag {
		verboseFlag = false
	}

	filenames := flag.Args()
	if len(filenames) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	filenames = dedupe(filenames)

	err := mvit(filenames)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
