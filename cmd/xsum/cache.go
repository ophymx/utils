package main

import (
	"encoding/binary"
	"os"
	"time"

	"github.com/ophymx/utils/attrutil"
	"github.com/ophymx/utils/xsum"
)

const xsumNS = "user.xsum"

type xattrCache struct {
	attrs attrutil.Attr
}

var _ xsum.Cache = (*xattrCache)(nil)

func newXattrCache() *xattrCache {
	return &xattrCache{attrutil.Xattr().NS(xsumNS)}
}

func timeToBytes(t time.Time) []byte {
	return binary.LittleEndian.AppendUint64(nil, uint64(t.Unix()))
}

func timeFromBytes(b []byte) time.Time {
	return time.Unix(int64(binary.LittleEndian.Uint64(b)), 0)
}

// Get returns the cached sums for the given filename.
// If the file has been modified since the last time the sums were cached, nil is returned.
func (c *xattrCache) Get(filename string) (map[string][]byte, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}

	b, err := c.attrs.Get(filename, "time")
	if err != nil {
		return nil, err
	}
	timestamp := timeFromBytes(b)

	if info.ModTime().After(timestamp) {
		c.attrs.DeleteNS(filename, "") // ignore error, this is just cleanup
		return nil, nil
	}

	keys, err := c.attrs.List(filename)
	if err != nil {
		return nil, err
	}
	sums := make(map[string][]byte)
	for _, key := range keys {
		if key == "time" {
			continue
		}
		if b, err := c.attrs.Get(filename, key); err == nil {
			sums[key] = b
		}
	}
	return sums, nil
}

// Set sets the cached sums for the given filename.
func (c *xattrCache) Set(filename string, sums map[string][]byte) error {
	for algorithm, sum := range sums {
		if err := c.attrs.Set(filename, algorithm, sum); err != nil {
			return err
		}
	}
	return c.attrs.Set(filename, "time", timeToBytes(time.Now()))
}
