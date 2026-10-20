package diskusage

import (
	"io/fs"
	"syscall"
)

type fileID struct{ device, inode uint64 }

// linkSet remembers files with several hard links, so the space they take is
// counted once.
type linkSet map[fileID]bool

// allocated returns the space info's item takes on disk: nothing for a
// further link to a file already counted. A nil set counts every link.
func (links linkSet) allocated(info fs.FileInfo) int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size()
	}
	if stat.Nlink > 1 && !info.IsDir() && links != nil {
		id := fileID{uint64(stat.Dev), uint64(stat.Ino)}
		if links[id] {
			return 0
		}
		links[id] = true
	}
	return int64(stat.Blocks) * 512
}

// HardLinks returns how many names the file info describes has (1 for a
// folder or where that isn't known).
func HardLinks(info fs.FileInfo) int {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.IsDir() {
		return 1
	}
	return int(stat.Nlink)
}
