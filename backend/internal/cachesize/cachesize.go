// Package cachesize keeps a cache folder under a size limit, removing the
// least recently used entries first.
package cachesize

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Entry is one removable part of a cache: a file, or a folder removed whole.
type Entry struct {
	Path     string
	Size     int64
	LastUsed time.Time
}

// Trim removes entries, least recently used first, until their total size is
// at most maxBytes (0 or less: no limit). Entries inUse reports true for are
// never removed. Returns the paths it removed.
func Trim(entries []Entry, maxBytes int64, inUse func(Entry) bool) []string {
	if maxBytes <= 0 {
		return nil
	}
	var total int64
	for _, entry := range entries {
		total += entry.Size
	}
	oldestFirst := append([]Entry(nil), entries...)
	sort.Slice(oldestFirst, func(left, right int) bool {
		return oldestFirst[left].LastUsed.Before(oldestFirst[right].LastUsed)
	})
	var removed []string
	for _, entry := range oldestFirst {
		if total <= maxBytes {
			break
		}
		if inUse != nil && inUse(entry) {
			continue
		}
		if err := os.RemoveAll(entry.Path); err != nil {
			log.Printf("cache: remove %s: %v", entry.Path, err)
			continue
		}
		total -= entry.Size
		removed = append(removed, entry.Path)
	}
	return removed
}

// Size returns the total size of the files at or under path.
func Size(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
