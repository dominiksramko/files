package handlers

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"files/internal/thumbs"
)

const convCacheTTL = 7 * 24 * time.Hour

type convCache struct {
	dir string
}

func newConvCache(dir string) (*convCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &convCache{dir: dir}
	go c.periodicCleanup()
	return c, nil
}

func (c *convCache) filePath(hash string) string {
	return filepath.Join(c.dir, hash+".jpg")
}

func (c *convCache) get(absPath string) ([]byte, string, bool) {
	hash, err := thumbs.FileContentHash(absPath)
	if err != nil {
		return nil, "", false
	}
	p := c.filePath(hash)
	info, err := os.Stat(p)
	if err != nil || time.Since(info.ModTime()) > convCacheTTL {
		return nil, hash, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, hash, false
	}
	return data, hash, true
}

func (c *convCache) put(hash string, data []byte) {
	if err := os.WriteFile(c.filePath(hash), data, 0o644); err != nil {
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
}

func (c *convCache) periodicCleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		c.cleanup()
	}
}
