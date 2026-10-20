//go:build !unix

package diskusage

import "io/fs"

type linkSet map[struct{}]bool

// allocated returns info's length where the space taken isn't known.
func (links linkSet) allocated(info fs.FileInfo) int64 { return info.Size() }

// HardLinks returns 1: the number of names isn't known here.
func HardLinks(info fs.FileInfo) int { return 1 }
