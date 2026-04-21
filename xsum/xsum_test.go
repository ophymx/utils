package xsum_test

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ophymx/utils/xsum"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newServer(t *testing.T, algorithms ...string) xsum.Server {
	t.Helper()
	srv, err := xsum.NewServer(algorithms...)
	if err != nil {
		t.Fatalf("NewServer(%v): %v", algorithms, err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}

func hex(b []byte) string {
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return string(dst)
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "xsum-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// ── NewServer ─────────────────────────────────────────────────────────────────

func TestNewServerEmpty(t *testing.T) {
	_, err := xsum.NewServer()
	if err == nil {
		t.Fatal("expected error for zero algorithms, got nil")
	}
}

func TestNewServerUnknownAlgorithm(t *testing.T) {
	_, err := xsum.NewServer("md5", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown algorithm, got nil")
	}
}

// ── single-algorithm hashers ──────────────────────────────────────────────────

var singleAlgoTests = []struct {
	algo string
	want func([]byte) []byte
}{
	{"md5", func(b []byte) []byte { s := md5.Sum(b); return s[:] }},
	{"sha256", func(b []byte) []byte { s := sha256.Sum256(b); return s[:] }},
	{"sha1", func(b []byte) []byte { s := sha1.Sum(b); return s[:] }},
	{"sha512", func(b []byte) []byte { s := sha512.Sum512(b); return s[:] }},
}

func TestSingleAlgoCorrectDigest(t *testing.T) {
	input := []byte("hello world")
	for _, tc := range singleAlgoTests {
		t.Run(tc.algo, func(t *testing.T) {
			srv := newServer(t, tc.algo)
			h := srv.NewHash()
			defer h.Close()

			if _, err := h.Write(input); err != nil {
				t.Fatalf("Write: %v", err)
			}
			sums := h.MultiSum()
			got, ok := sums[tc.algo]
			if !ok {
				t.Fatalf("MultiSum missing key %q, got keys: %v", tc.algo, keys(sums))
			}
			if want := tc.want(input); string(got) != string(want) {
				t.Errorf("got %s, want %s", hex(got), hex(want))
			}
			if len(sums) != 1 {
				t.Errorf("expected 1 key in MultiSum, got %d: %v", len(sums), keys(sums))
			}
		})
	}
}

func TestSingleAlgoChunkedWrite(t *testing.T) {
	input := "the quick brown fox jumps over the lazy dog"
	for _, tc := range singleAlgoTests {
		t.Run(tc.algo, func(t *testing.T) {
			srv := newServer(t, tc.algo)
			h := srv.NewHash()
			defer h.Close()

			for chunk := range strings.FieldsSeq(input) {
				if _, err := h.Write([]byte(chunk)); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}
			sums := h.MultiSum()
			got := sums[tc.algo]
			// chunked without spaces — compare to same concatenation
			want := tc.want([]byte(strings.Join(strings.Fields(input), "")))
			if string(got) != string(want) {
				t.Errorf("got %s, want %s", hex(got), hex(want))
			}
		})
	}
}

func TestSingleAlgoReset(t *testing.T) {
	input := []byte("clean")
	for _, tc := range singleAlgoTests {
		t.Run(tc.algo, func(t *testing.T) {
			srv := newServer(t, tc.algo)
			h := srv.NewHash()
			defer h.Close()

			if _, err := h.Write([]byte("noise")); err != nil {
				t.Fatal(err)
			}
			h.Reset()
			if _, err := h.Write(input); err != nil {
				t.Fatal(err)
			}
			got := h.MultiSum()[tc.algo]
			want := tc.want(input)
			if string(got) != string(want) {
				t.Errorf("after Reset: got %s, want %s", hex(got), hex(want))
			}
		})
	}
}

// ── multi-algorithm hasher ────────────────────────────────────────────────────

func TestMultiHasherCorrectDigests(t *testing.T) {
	input := []byte("hello world")
	srv := newServer(t, "md5", "sha256")
	h := srv.NewHash()
	defer h.Close()

	if _, err := h.Write(input); err != nil {
		t.Fatalf("Write: %v", err)
	}
	sums := h.MultiSum()

	wantMD5 := md5.Sum(input)
	wantSHA256 := sha256.Sum256(input)

	if got, ok := sums["md5"]; !ok || string(got) != string(wantMD5[:]) {
		t.Errorf("md5: got %s, want %s", hex(sums["md5"]), hex(wantMD5[:]))
	}
	if got, ok := sums["sha256"]; !ok || string(got) != string(wantSHA256[:]) {
		t.Errorf("sha256: got %s, want %s", hex(sums["sha256"]), hex(wantSHA256[:]))
	}
	if len(sums) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(sums), keys(sums))
	}
}

func TestMultiHasherBlockSizeLCM(t *testing.T) {
	// md5 block=64, sha256 block=64 → LCM=64
	// sha512 block=128 → LCM(64,128)=128
	srv := newServer(t, "md5", "sha512")
	h := srv.NewHash()
	defer h.Close()
	if got := h.BlockSize(); got != 128 {
		t.Errorf("BlockSize() = %d, want 128", got)
	}
}

func TestMultiHasherReset(t *testing.T) {
	input := []byte("clean")
	srv := newServer(t, "md5", "sha256")
	h := srv.NewHash()
	defer h.Close()

	if _, err := h.Write([]byte("noise")); err != nil {
		t.Fatal(err)
	}
	h.Reset()
	if _, err := h.Write(input); err != nil {
		t.Fatal(err)
	}
	sums := h.MultiSum()

	wantMD5 := md5.Sum(input)
	wantSHA256 := sha256.Sum256(input)
	if string(sums["md5"]) != string(wantMD5[:]) {
		t.Errorf("md5 after Reset: got %s, want %s", hex(sums["md5"]), hex(wantMD5[:]))
	}
	if string(sums["sha256"]) != string(wantSHA256[:]) {
		t.Errorf("sha256 after Reset: got %s, want %s", hex(sums["sha256"]), hex(wantSHA256[:]))
	}
}

// ── Parallel ──────────────────────────────────────────────────────────────────

func TestParallelCorrectSums(t *testing.T) {
	inputs := map[string]string{
		"a": "foo",
		"b": "bar",
		"c": "baz",
	}
	files := make([]string, 0, len(inputs))
	dir := t.TempDir()
	for name, content := range inputs {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}

	srv := newServer(t, "md5", "sha256")

	type got struct {
		sums map[string][]byte
		err  error
	}
	results := make(map[string]got)
	var mu sync.Mutex
	xsum.Parallel(context.Background(), srv, nil, files, func(filename string, sums map[string][]byte, err error) {
		mu.Lock()
		results[filename] = got{sums, err}
		mu.Unlock()
	})

	for name, content := range inputs {
		path := filepath.Join(dir, name)
		r, ok := results[path]
		if !ok {
			t.Errorf("no result for %s", name)
			continue
		}
		if r.err != nil {
			t.Errorf("%s: unexpected error: %v", name, r.err)
			continue
		}
		b := []byte(content)
		wantMD5 := md5.Sum(b)
		wantSHA256 := sha256.Sum256(b)
		if string(r.sums["md5"]) != string(wantMD5[:]) {
			t.Errorf("%s md5: got %s, want %s", name, hex(r.sums["md5"]), hex(wantMD5[:]))
		}
		if string(r.sums["sha256"]) != string(wantSHA256[:]) {
			t.Errorf("%s sha256: got %s, want %s", name, hex(r.sums["sha256"]), hex(wantSHA256[:]))
		}
	}
}

func TestParallelMissingFile(t *testing.T) {
	srv := newServer(t, "sha256")
	var gotErr error
	xsum.Parallel(context.Background(), srv, nil, []string{"/nonexistent/file"}, func(_ string, _ map[string][]byte, err error) {
		gotErr = err
	})
	if gotErr == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ── Cache ─────────────────────────────────────────────────────────────────────

type memCache struct {
	mu   sync.Mutex
	data map[string]map[string][]byte
	hits int
}

func newMemCache() *memCache { return &memCache{data: make(map[string]map[string][]byte)} }

func (c *memCache) Get(filename string) (map[string][]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sums, ok := c.data[filename]; ok {
		c.hits++
		return sums, nil
	}
	return nil, nil
}

func (c *memCache) Set(filename string, sums map[string][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[filename] = sums
	return nil
}

func TestParallelCacheMissPopulatesCache(t *testing.T) {
	path := writeTempFile(t, "hello")
	srv := newServer(t, "sha256")
	cache := newMemCache()

	xsum.Parallel(context.Background(), srv, cache, []string{path}, func(_ string, _ map[string][]byte, err error) {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if _, ok := cache.data[path]; !ok {
		t.Error("cache not populated after miss")
	}
	if cache.hits != 0 {
		t.Errorf("expected 0 cache hits on first pass, got %d", cache.hits)
	}
}

func TestParallelCacheHitSkipsRecompute(t *testing.T) {
	path := writeTempFile(t, "hello")
	srv := newServer(t, "sha256")
	cache := newMemCache()

	sentinel := map[string][]byte{"sha256": []byte("sentinel-value")}
	cache.data[path] = sentinel

	var gotSums map[string][]byte
	xsum.Parallel(context.Background(), srv, cache, []string{path}, func(_ string, sums map[string][]byte, err error) {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		gotSums = sums
	})

	if cache.hits != 1 {
		t.Errorf("expected 1 cache hit, got %d", cache.hits)
	}
	if string(gotSums["sha256"]) != "sentinel-value" {
		t.Error("cache hit did not return cached value")
	}
}

func TestParallelContextCancellation(t *testing.T) {
	// Write enough temp files to keep workers busy.
	n := 20
	files := make([]string, n)
	for i := range n {
		files[i] = writeTempFile(t, strings.Repeat("x", 1<<16))
	}

	srv := newServer(t, "sha256")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel: producer should enqueue nothing

	var count int
	xsum.Parallel(ctx, srv, nil, files, func(_ string, _ map[string][]byte, _ error) {
		count++
	})

	if count != 0 {
		t.Errorf("expected 0 files processed after pre-cancellation, got %d", count)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func keys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
