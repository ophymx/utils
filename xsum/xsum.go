package xsum

import (
	"crypto/sha1"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/klauspost/cpuid/v2"
	md5simd "github.com/minio/md5-simd"
	sha256simd "github.com/minio/sha256-simd"
)

type Cache interface {
	// Get returns the hash sums for a given file.
	Get(filename string) (sums map[string][]byte, err error)
	// Set stores the hash sums for a given file.
	Set(filename string, sums map[string][]byte) (err error)
}

// OnResult is a callback function type that is called when a file's hash computation is done.
type OnResult func(filename string, sums map[string][]byte, err error)

// Server defines an interface for a hash computation server.
type Server interface {
	NewHash() Hash
	Parallel(filenames []string, onResult OnResult)
	Close() error
}

var hasAvx512 = cpuid.CPU.Supports(cpuid.AVX512F, cpuid.AVX512DQ, cpuid.AVX512BW, cpuid.AVX512VL)

// NewHashFunc is a function type that returns a new hash.Hash.
type NewHashFunc func() hash.Hash

type serverImpl struct {
	cache      Cache
	algorithms map[string]NewHashFunc
	closers    []func() error
}

// NewServer creates a new Server with the specified algorithms.
func NewServer(cache Cache, algorithms ...string) (Server, error) {
	srv := &serverImpl{cache: cache}
	srv.algorithms = make(map[string]NewHashFunc)

	for _, algorithm := range algorithms {
		switch algorithm {
		case "md5":
			md5Srv := md5simd.NewServer()
			srv.algorithms["md5"] = func() hash.Hash {
				return md5Srv.NewHash()
			}
			srv.closers = append(srv.closers, func() error {
				md5Srv.Close()
				return nil
			})
		case "sha256":
			if hasAvx512 {
				sha256Srv := sha256simd.NewAvx512Server()
				srv.algorithms["sha256"] = func() hash.Hash {
					return sha256simd.NewAvx512(sha256Srv)
				}
			} else {
				srv.algorithms["sha256"] = func() hash.Hash {
					return sha256simd.New()
				}
			}
		case "sha1":
			srv.algorithms["sha1"] = sha1.New
		case "sha512":
			srv.algorithms["sha512"] = sha512.New
		default:
			return nil, fmt.Errorf("unknown algorithm: %s", algorithm)
		}
	}
	return srv, nil
}

type result struct {
	filename string
	sums     map[string][]byte
	err      error
}

// hashFile computes the hash sums for a given file using the provided Hash.
func hashFile(filename string, hash Hash) (sums map[string][]byte, err error) {
	if f, err := os.Open(filename); err != nil {
		return nil, err
	} else {
		defer f.Close()
		_, err := io.Copy(hash, f)
		if err != nil {
			return nil, err
		}
	}
	return hash.MultiSum(nil), nil
}

// Parallel computes the hash sums for multiple files in parallel.
func (srv *serverImpl) Parallel(filenames []string, onResult OnResult) {
	numWorkers := srv.NumberOfWorkers()

	fileChan := make(chan string, numWorkers)
	resultChan := make(chan *result)
	done := make(chan bool)

	go func() {
		for _, filename := range filenames {
			fileChan <- filename
		}
		close(fileChan)
	}()

	go func() {
		for result := range resultChan {
			onResult(result.filename, result.sums, result.err)
		}
		done <- true
	}()

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filename := range fileChan {
				if srv.cache != nil {
					if sums, err := srv.cache.Get(filename); err == nil {
						missing := false
						for algorithm := range srv.algorithms {
							if _, ok := sums[algorithm]; !ok {
								missing = true
								break
							}
						}
						if !missing {
							resultChan <- &result{filename, sums, err}
							continue
						}
					}
				}
				sums, err := hashFile(filename, srv.NewHash())
				if srv.cache != nil && err == nil {
					err = srv.cache.Set(filename, sums)
				}
				resultChan <- &result{filename, sums, err}
			}
		}()
	}
	wg.Wait()
	close(resultChan)
	<-done
}

// NumberOfWorkers returns the number of workers to use for parallel processing.
func (srv *serverImpl) NumberOfWorkers() int {
	num := runtime.NumCPU() / len(srv.algorithms)
	if num > 16 {
		return 16
	}
	return 1
}

// Close closes the server and releases any resources.
func (srv *serverImpl) Close() error {
	var err error
	for _, closer := range srv.closers {
		if err2 := closer(); err2 != nil {
			if err == nil {
				err = err2
			} else {
				err = fmt.Errorf("%v; %v", err, err2)
			}
		}
	}
	srv.algorithms = nil
	srv.closers = nil
	return err
}

// NewHash creates a new Hash that computes multiple hash sums.
func (s *serverImpl) NewHash() Hash {
	digests := make(map[string]hash.Hash)
	for algorithm, newHash := range s.algorithms {
		digests[algorithm] = newHash()
	}

	return &hashImpl{digests}
}

// Hash is an interface that extends hash.Hash with a method to compute multiple hash sums.
type Hash interface {
	hash.Hash
	MultiSum(data []byte) (sums map[string][]byte)
}

type hashImpl struct {
	digests map[string]hash.Hash
}

var _ Hash = new(hashImpl)

// MultiSum computes the hash sums for the given data.
func (h *hashImpl) MultiSum(data []byte) (sums map[string][]byte) {
	sums = make(map[string][]byte)
	for algorithm, digest := range h.digests {
		sums[algorithm] = digest.Sum(data)
	}
	return
}

// Sum computes the combined hash sum for the given data.
func (h *hashImpl) Sum(b []byte) []byte {
	sums := h.MultiSum(b)
	ret := make([]byte, 0, h.Size())
	for _, sum := range sums {
		ret = append(ret, sum...)
	}
	return ret
}

// Write writes data to all underlying hash functions.
func (h *hashImpl) Write(p []byte) (n int, err error) {
	for _, digest := range h.digests {
		n, err = digest.Write(p)
		if err != nil {
			return // this should never happen, but just in case...
		}
	}
	return
}

// Reset resets all underlying hash functions.
func (h *hashImpl) Reset() {
	for _, digest := range h.digests {
		digest.Reset()
	}
}

// Size returns the combined size of all hash sums.
func (h *hashImpl) Size() int {
	total := 0
	for _, digest := range h.digests {
		total += digest.Size()
	}
	return total
}

// BlockSize returns the maximum block size of all underlying hash functions.
func (h *hashImpl) BlockSize() int {
	max := 0
	for _, digest := range h.digests {
		if digest.BlockSize() > max {
			max = digest.BlockSize()
		}
	}
	return max
}
