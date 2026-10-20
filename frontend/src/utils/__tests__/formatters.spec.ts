import { describe, expect, it } from 'vitest'
import { formatDiskProgress, formatDuration, formatSizeProgress, formatTimeAgo } from '@/utils/formatters'

describe('formatDuration', () => {
  it('shows minutes and seconds, hours from an hour on', () => {
    expect(formatDuration(0)).toBe('0:00')
    expect(formatDuration(65.9)).toBe('1:05')
    expect(formatDuration(3600 + 2 * 60 + 3)).toBe('1:02:03')
  })
})

describe('formatSizeProgress', () => {
  const progress = { files: 3, dirs: 1, bytes: 2048, disk: 12288, done: false }

  it('says it is still scanning', () => {
    expect(formatSizeProgress(progress)).toBe('2.0 KB (scanning… 3 files, 1 folder)')
    expect(formatSizeProgress({ files: 0, dirs: 0, bytes: 0, disk: 0, done: false })).toBe('(scanning…)')
  })

  it('shows the totals once done', () => {
    expect(formatSizeProgress({ ...progress, done: true })).toBe('2.0 KB (3 files, 1 folder)')
    expect(formatSizeProgress({ files: 1, dirs: 0, bytes: 0, disk: 0, done: true })).toBe('0 B (1 file)')
    expect(formatSizeProgress({ files: 0, dirs: 0, bytes: 0, disk: 4096, done: true })).toBe('Empty')
  })

  it('says a scan that stopped early is incomplete', () => {
    expect(formatSizeProgress({ ...progress, failed: true })).toBe('2.0 KB (incomplete: 3 files, 1 folder)')
    expect(formatSizeProgress({ files: 0, dirs: 0, bytes: 0, disk: 0, done: false, failed: true })).toBe("Couldn't measure")
  })
})

describe('formatDiskProgress', () => {
  const progress = { files: 3, dirs: 1, bytes: 2048, disk: 12288, done: false }

  it('shows the space on disk, marked while still scanning', () => {
    expect(formatDiskProgress(progress)).toBe('12.0 KB…')
    expect(formatDiskProgress({ ...progress, disk: 0 })).toBe('scanning…')
    expect(formatDiskProgress({ ...progress, done: true })).toBe('12.0 KB')
    expect(formatDiskProgress({ ...progress, disk: 0, done: true })).toBe('0 B')
  })

  it('says a scan that stopped early is incomplete', () => {
    expect(formatDiskProgress({ ...progress, failed: true })).toBe('12.0 KB (incomplete)')
    expect(formatDiskProgress({ ...progress, disk: 0, failed: true })).toBe("Couldn't measure")
  })
})

describe('formatTimeAgo', () => {
  const now = Date.parse('2026-10-20T12:00:00Z')
  it('says how long ago', () => {
    expect(formatTimeAgo('2026-10-20T11:59:40Z', now)).toBe('just now')
    expect(formatTimeAgo('2026-10-20T11:55:00Z', now)).toBe('5 min ago')
    expect(formatTimeAgo('2026-10-20T09:00:00Z', now)).toBe('3 h ago')
    expect(formatTimeAgo('2026-10-19T12:00:00Z', now)).toBe('1 day ago')
    expect(formatTimeAgo('2026-10-17T11:00:00Z', now)).toBe('3 days ago')
  })
})
