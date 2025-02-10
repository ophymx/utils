package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ophymx/utils/xsum"
)

var (
	helpFlag      bool
	cacheFlag     bool
	versionFlag   bool
	verboseFlag   bool
	outputFlag    string
	algorithmFlag string
)

const version = "0.1"

func init() {
	flag.BoolVar(&helpFlag, "h", false, "Display help")
	flag.BoolVar(&cacheFlag, "c", true, "Use cache")
	flag.BoolVar(&versionFlag, "V", false, "Display version")
	flag.BoolVar(&verboseFlag, "v", false, "Verbose output")
	flag.StringVar(&outputFlag, "f", "csv", "Output format (csv, json)")
	flag.StringVar(&algorithmFlag, "a", "sha256,md5", "Algorithms (comma separated)")
	flag.Usage = func() {
		println("Usage: xsum [options] file1 file2 ...")
		println()
		println("xsum - calculate checksums of files in parallel")
		println()
		flag.PrintDefaults()
	}
}

type xsumWriter interface {
	io.Closer
	Write(hostname string, filename string, size int64, sums map[string][]byte, err error) error
}

var writers = map[string]func(w io.Writer, algorithms []string) xsumWriter{
	"json": func(w io.Writer, algorithms []string) xsumWriter {
		return &jsonWriter{w}
	},
	"csv": func(w io.Writer, algorithms []string) xsumWriter {
		return newCsvWriter(w, algorithms)
	},
}

func cache() xsum.Cache {
	if cacheFlag {
		return newXattrCache()
	}
	return nil
}

func doXsum(filenames []string, algorithms []string) (err error) {
	newWriter, ok := writers[outputFlag]
	if !ok {
		return fmt.Errorf("unknown output format: %s", outputFlag)
	}
	writer := newWriter(os.Stdout, algorithms)
	defer writer.Close()

	var hostname string
	if hostname, err = os.Hostname(); err != nil {
		return
	}
	var sizes = make(map[string]int64, len(filenames))
	for i, filename := range filenames {
		if filename, err = filepath.Abs(filename); err != nil {
			return
		}
		filenames[i] = filename
		var info fs.FileInfo
		if info, err = os.Stat(filename); err != nil {
			return
		}
		sizes[filename] = info.Size()
	}

	srv, err := xsum.NewServer(cache(), algorithms...)
	if err != nil {
		return
	}
	srv.Parallel(filenames, func(filename string, sums map[string][]byte, err error) {
		if e := writer.Write(hostname, filename, sizes[filename], sums, err); e != nil {
			log.Panicln(e)
		}
	})

	return
}

func main() {
	flag.Parse()
	if helpFlag {
		flag.Usage()
		os.Exit(0)
	}

	if versionFlag {
		println(version)
		os.Exit(0)
	}

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	if err := doXsum(flag.Args(), strings.Split(algorithmFlag, ",")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
