//go:build linux

package handlers

import (
	"time"

	"golang.org/x/sys/unix"
)

func getBirthTime(path string) *time.Time {
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_BTIME, &stat); err != nil {
		return nil
	}
	if stat.Mask&unix.STATX_BTIME == 0 || (stat.Btime.Sec == 0 && stat.Btime.Nsec == 0) {
		return nil
	}
	t := time.Unix(int64(stat.Btime.Sec), int64(stat.Btime.Nsec))
	return &t
}
