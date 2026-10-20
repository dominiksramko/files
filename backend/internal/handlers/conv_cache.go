package handlers

import (
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"files/internal/cachesize"
	"files/internal/thumbs"
)

const convCacheTTL = 7 * 24 * time.Hour

type convCache struct {
	dir string
	// maxBytes: size limit of the cache (0 = none); the oldest conversions go first.
	maxBytes atomic.Int64
}

func newConvCache(dir string) (*convCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &convCache{dir: dir}
	go c.periodicCleanup()
	return c, nil
}

// filePath returns the cache location for a conversion of the given content
// hash; ext (".jpg", ".m4a") tells apart conversions of different kinds.
func (c *convCache) filePath(hash, ext string) string {
	return filepath.Join(c.dir, hash+ext)
}

// lookup hashes absPath and returns its cache location. ok reports whether a
// fresh conversion is already there; hash is empty if the source can't be read.
func (c *convCache) lookup(absPath, ext string) (hash, cachedPath string, ok bool) {
	hash, err := thumbs.FileContentHash(absPath)
	if err != nil {
		return "", "", false
	}
	cachedPath = c.filePath(hash, ext)
	info, err := os.Stat(cachedPath)
	return hash, cachedPath, err == nil && time.Since(info.ModTime()) <= convCacheTTL
}

func (c *convCache) get(absPath, ext string) ([]byte, string, bool) {
	hash, cachedPath, ok := c.lookup(absPath, ext)
	if !ok {
		return nil, hash, false
	}
	data, err := os.ReadFile(cachedPath)
	if err != nil {
		return nil, hash, false
	}
	return data, hash, true
}

func (c *convCache) put(hash, ext string, data []byte) {
	if err := os.WriteFile(c.filePath(hash, ext), data, 0o644); err != nil {
		log.Printf("conv cache: write %s: %v", hash, err)
	}
}

func (c *convCache) cleanup() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	deleted := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > convCacheTTL {
			os.Remove(filepath.Join(c.dir, e.Name()))
			deleted++
		}
	}
	if deleted > 0 {
		log.Printf("conv cache: removed %d expired entries", deleted)
	}
	c.trim()
}

// trim removes the oldest conversions until the cache fits in maxBytes.
func (c *convCache) trim() {
	files, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	var entries []cachesize.Entry
	for _, file := range files {
		if info, err := file.Info(); err == nil && info.Mode().IsRegular() {
			entries = append(entries, cachesize.Entry{Path: filepath.Join(c.dir, file.Name()), Size: info.Size(), LastUsed: info.ModTime()})
		}
	}
	if removed := cachesize.Trim(entries, c.maxBytes.Load(), nil); len(removed) > 0 {
		log.Printf("conv cache: removed %d entries over the size limit", len(removed))
	}
}

func (c *convCache) periodicCleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		c.cleanup()
	}
}
