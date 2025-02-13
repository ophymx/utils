package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	helpFlag    bool
	versionFlag bool
	listenFlag  string
	keyFlag     string
	certFlag    string
)

const (
	usage       = "Usage: ohttpd [options] [mountpoint:][source] ..."
	description = "quick and dirty HTTP server"
	version     = "0.1"
)

// init initializes the command-line flags and usage message.
func init() {
	flag.BoolVar(&helpFlag, "h", false, "Display help")
	flag.StringVar(&listenFlag, "l", ":8080", "Listen address")
	flag.StringVar(&keyFlag, "k", "", "TLS key file (requires -c)")
	flag.StringVar(&certFlag, "c", "", "TLS certificate file (requires -k)")
	flag.BoolVar(&versionFlag, "V", false, "Display version")
	flag.Usage = func() {
		fmt.Println(usage)
		fmt.Println(description)
		flag.PrintDefaults()
	}
}

// parseURI parses a URI string and returns a URL object.
func parseURI(uri string) (u *url.URL, err error) {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "file://") {
		return url.Parse(uri)
	}
	if !filepath.IsAbs(uri) {
		uri, err = filepath.Abs(uri)
		if err != nil {
			return nil, err
		}
	}
	return &url.URL{Scheme: "file", Path: uri}, nil
}

// Mount represents a mount point with a path, source URL, and rewrite flag.
type Mount struct {
	Path    string
	Source  *url.URL
	Rewrite bool
}

// mount mounts the handler to the given ServeMux.
func (m *Mount) mount(mux *http.ServeMux) {
	var handler http.Handler
	if m.Source.Scheme == "file" {
		handler = http.FileServer(http.Dir(m.Source.Path))
	} else {
		handler = httputil.NewSingleHostReverseProxy(m.Source)
	}
	if m.Rewrite {
		handler = http.StripPrefix(m.Path, handler)
	}
	log.Printf("Mounting %s at %s", m.Source, m.Path)
	mux.Handle(m.Path, handler)
}

// parseMount parses a mount string and returns a Mount struct.
// example: "/path:http://example.com"
// example: "/path:/local/path"
// example: "/path:file:///local/path"
// example: "."
// example: "http://example.com"
// example: "file:///local/path"
// example: "/local/path"
// example: "/path:-http://example.com" -> rewrite /path as / on upstream.
func parseMount(mount string) (mnt *Mount, err error) {
	var path string
	var source *url.URL
	var rewrite bool
	if strings.HasPrefix(mount, "http://") || strings.HasPrefix(mount, "https://") || strings.HasPrefix(mount, "file://") {
		source, err = url.Parse(mount)
		if err != nil {
			return nil, err
		}
		path = "/"
		return &Mount{Path: path, Source: source}, nil
	}

	parts := strings.SplitN(mount, ":", 2)
	if len(parts) == 1 {
		path = "/"
		source, err = parseURI(parts[0])
	} else {
		path = parts[0]
		if strings.HasPrefix(parts[1], "-") {
			rewrite = true
			parts[1] = strings.TrimPrefix(parts[1], "-")
		}
		source, err = parseURI(parts[1])
	}

	if err != nil {
		return nil, err
	}

	return &Mount{
		Path:    path,
		Source:  source,
		Rewrite: rewrite,
	}, nil
}

var validSourceSchemes = map[string]bool{
	"http":  true,
	"https": true,
	"file":  true,
}

// parseMounts parses multiple mount options and returns a map of Mount structs.
func parseMounts(mountOptions []string) (mounts map[string]*Mount, err error) {
	mounts = make(map[string]*Mount)

	for _, mountOption := range mountOptions {
		mnt, err := parseMount(mountOption)
		if err != nil {
			return nil, err
		}
		if !validSourceSchemes[mnt.Source.Scheme] {
			return nil, fmt.Errorf("invalid source scheme %s", mnt.Source.Scheme)
		}

		if _, ok := mounts[mnt.Path]; ok {
			return nil, fmt.Errorf("duplicate mount point %s", mnt.Path)
		}

		mounts[mnt.Path] = mnt
	}
	return mounts, nil
}

func main() {
	flag.Parse()

	if helpFlag {
		flag.Usage()
		return
	}

	if versionFlag {
		fmt.Printf("ohttp version %s\n", version)
		return
	}

	if keyFlag != "" && certFlag == "" {
		fmt.Println("Error: -c must be specified if -k is specified")
		flag.Usage()
		os.Exit(1)
	}

	if certFlag != "" && keyFlag == "" {
		fmt.Println("Error: -k must be specified if -c is specified")
		flag.Usage()
		os.Exit(1)
		return
	}

	mountArgs := flag.Args()
	if len(mountArgs) == 0 {
		mountArgs = append(mountArgs, ".")
	}

	mounts, err := parseMounts(mountArgs)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		flag.Usage()
		os.Exit(1)
	}

	if err := serve(mounts); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}

// logHandler logs HTTP requests and passes them to the next handler.
func logHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}

// serve starts the HTTP server with the given mounts.
func serve(mounts map[string]*Mount) error {
	// Create a new HTTP server
	mux := http.NewServeMux()
	// Add a handler for each mount point
	for _, mnt := range mounts {
		mnt.mount(mux)
	}

	server := &http.Server{
		Addr:    listenFlag,
		Handler: logHandler(mux),
	}

	log.Printf("Listening on %s", listenFlag)
	// Start the server
	if keyFlag != "" && certFlag != "" {
		return server.ListenAndServeTLS(certFlag, keyFlag)
	}
	return server.ListenAndServe()
}
