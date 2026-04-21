package xsum

import (
	"crypto/sha1"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"maps"
	"os"
	"runtime"
	"sync"

	"github.com/klauspost/cpuid/v2"
	md5simd "github.com/minio/md5-simd"
	sha256simd "github.com/minio/sha256-simd"
)

// Hasher is a multi-algorithm hash that writes data once and returns per-algorithm
// digests via MultiSum. Sum and Size from hash.Hash are intentionally omitted —
// use MultiSum instead.
type Hasher interface {
	io.Writer
	Reset()
	BlockSize() int
	Close()
	MultiSum() map[string][]byte
}

// Server creates Hasher instances and owns any underlying SIMD servers.
type Server interface {
	NewHash() Hasher
	Close() error
}

// Cache is an optional read/write store for previously computed sums.
// Any miss (empty map or error) causes a full recompute of all algorithms.
type Cache interface {
	Get(filename string) (map[string][]byte, error)
	Set(filename string, sums map[string][]byte) error
}

// OnResult is called when a file's hash computation is done.
type OnResult func(filename string, sums map[string][]byte, err error)

// ── md5 leaf ──────────────────────────────────────────────────────────────────

type md5Hasher struct{ md5simd.Hasher }

func (h *md5Hasher) MultiSum() map[string][]byte {
	return map[string][]byte{"md5": h.Sum(nil)}
}

type md5Server struct{ srv md5simd.Server }

func newMD5Server() *md5Server       { return &md5Server{srv: md5simd.NewServer()} }
func (s *md5Server) NewHash() Hasher { return &md5Hasher{s.srv.NewHash()} }
func (s *md5Server) Close() error    { s.srv.Close(); return nil }

// ── sha256 leaf ───────────────────────────────────────────────────────────────

var hasAvx512 = cpuid.CPU.Supports(cpuid.AVX512F, cpuid.AVX512DQ, cpuid.AVX512BW, cpuid.AVX512VL)

type sha256Hasher struct{ hash.Hash }

func (h *sha256Hasher) Close() {}
func (h *sha256Hasher) MultiSum() map[string][]byte {
	return map[string][]byte{"sha256": h.Sum(nil)}
}

type sha256Server struct{ avx512 *sha256simd.Avx512Server }

func newSHA256Server() *sha256Server {
	if hasAvx512 {
		return &sha256Server{avx512: sha256simd.NewAvx512Server()}
	}
	return &sha256Server{}
}

func (s *sha256Server) NewHash() Hasher {
	if s.avx512 != nil {
		return &sha256Hasher{sha256simd.NewAvx512(s.avx512)}
	}
	return &sha256Hasher{sha256simd.New()}
}

func (s *sha256Server) Close() error { return nil }

// ── stdlib leaf (sha1, sha512) ────────────────────────────────────────────────

type stdHasher struct {
	hash.Hash
	name string
}

func (h *stdHasher) Close() {}
func (h *stdHasher) MultiSum() map[string][]byte {
	return map[string][]byte{h.name: h.Sum(nil)}
}

type stdServer struct {
	name    string
	newHash func() hash.Hash
}

func (s *stdServer) NewHash() Hasher { return &stdHasher{s.newHash(), s.name} }
func (s *stdServer) Close() error    { return nil }

// ── multi hasher + server ─────────────────────────────────────────────────────

type multiHasher struct {
	io.Writer
	hashers   []Hasher
	blockSize int
}

func newMultiHasher(hashers []Hasher) *multiHasher {
	writers := make([]io.Writer, len(hashers))
	blockSize := 1
	for i, h := range hashers {
		writers[i] = h
		blockSize = lcm(blockSize, h.BlockSize())
	}
	return &multiHasher{Writer: io.MultiWriter(writers...), hashers: hashers, blockSize: blockSize}
}

func (h *multiHasher) Close() {
	for _, hasher := range h.hashers {
		hasher.Close()
	}
}

func (h *multiHasher) MultiSum() map[string][]byte {
	sums := make(map[string][]byte)
	for _, hasher := range h.hashers {
		maps.Copy(sums, hasher.MultiSum())
	}
	return sums
}

func (h *multiHasher) Reset() {
	for _, hasher := range h.hashers {
		hasher.Reset()
	}
}

func (h *multiHasher) BlockSize() int { return h.blockSize }

type multiServer struct{ servers []Server }

func (s *multiServer) NewHash() Hasher {
	hashers := make([]Hasher, len(s.servers))
	for i, srv := range s.servers {
		hashers[i] = srv.NewHash()
	}
	return newMultiHasher(hashers)
}

func (s *multiServer) Close() error {
	var errs []error
	for _, srv := range s.servers {
		if err := srv.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ── construction ──────────────────────────────────────────────────────────────

// NewServer creates a Server for the named algorithms ("md5", "sha256", "sha1", "sha512").
// A single algorithm returns a leaf Server directly; multiple algorithms return a multiServer.
func NewServer(algorithms ...string) (Server, error) {
	if len(algorithms) == 0 {
		return nil, errors.New("at least one algorithm is required")
	}
	servers := make([]Server, 0, len(algorithms))
	for _, algorithm := range algorithms {
		switch algorithm {
		case "md5":
			servers = append(servers, newMD5Server())
		case "sha256":
			servers = append(servers, newSHA256Server())
		case "sha1":
			servers = append(servers, &stdServer{"sha1", sha1.New})
		case "sha512":
			servers = append(servers, &stdServer{"sha512", sha512.New})
		default:
			for _, s := range servers {
				s.Close()
			}
			return nil, fmt.Errorf("unknown algorithm: %s", algorithm)
		}
	}
	if len(servers) == 1 {
		return servers[0], nil
	}
	return &multiServer{servers: servers}, nil
}

// ── file hashing + parallel ───────────────────────────────────────────────────

type result struct {
	filename string
	sums     map[string][]byte
	err      error
}

func hashFile(filename string, h Hasher) (map[string][]byte, error) {
	defer h.Close()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err = io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.MultiSum(), nil
}

const maxWorkers = 16

func numWorkers() int {
	n := runtime.NumCPU()
	if n > maxWorkers {
		return maxWorkers
	}
	if n < 1 {
		return 1
	}
	return n
}

// Parallel computes hash sums for multiple files concurrently.
// If cache is non-nil it is consulted before hashing and updated after.
func Parallel(srv Server, cache Cache, filenames []string, onResult OnResult) {
	nw := numWorkers()

	fileChan := make(chan string, nw)
	resultChan := make(chan *result, nw)
	done := make(chan struct{})

	go func() {
		for _, filename := range filenames {
			fileChan <- filename
		}
		close(fileChan)
	}()

	go func() {
		for r := range resultChan {
			onResult(r.filename, r.sums, r.err)
		}
		close(done)
	}()

	var wg sync.WaitGroup
	for range nw {
		wg.Go(func() {
			for filename := range fileChan {
				if cache != nil {
					if sums, err := cache.Get(filename); err == nil && len(sums) > 0 {
						resultChan <- &result{filename, sums, nil}
						continue
					}
				}
				sums, err := hashFile(filename, srv.NewHash())
				if cache != nil && err == nil {
					_ = cache.Set(filename, sums)
				}
				resultChan <- &result{filename, sums, err}
			}
		})
	}
	wg.Wait()
	close(resultChan)
	<-done
}

// ── math helpers ──────────────────────────────────────────────────────────────

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func lcm(a, b int) int { return a / gcd(a, b) * b }
