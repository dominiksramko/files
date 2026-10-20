import { describe, expect, it } from 'vitest'
import { formatDuration, formatSizeProgress } from '@/utils/formatters'

describe('formatDuration', () => {
  it('shows minutes and seconds, hours from an hour on', () => {
    expect(formatDuration(0)).toBe('0:00')
    expect(formatDuration(65.9)).toBe('1:05')
    expect(formatDuration(3600 + 2 * 60 + 3)).toBe('1:02:03')
  })
})

describe('formatSizeProgress', () => {
  const progress = { files: 3, dirs: 1, bytes: 2048, done: false }

  it('says it is still scanning', () => {
    expect(formatSizeProgress(progress)).toBe('2.0 KB (scanning… 3 files, 1 folder)')
    expect(formatSizeProgress({ files: 0, dirs: 0, bytes: 0, done: false })).toBe('(scanning…)')
  })

  it('shows the totals once done', () => {
    expect(formatSizeProgress({ ...progress, done: true })).toBe('2.0 KB (3 files, 1 folder)')
    expect(formatSizeProgress({ files: 1, dirs: 0, bytes: 0, done: true })).toBe('0 B (1 file)')
    expect(formatSizeProgress({ files: 0, dirs: 0, bytes: 0, done: true })).toBe('Empty')
  })

  it('says a scan that stopped early is incomplete', () => {
    expect(formatSizeProgress({ ...progress, failed: true })).toBe('2.0 KB (incomplete: 3 files, 1 folder)')
    expect(formatSizeProgress({ files: 0, dirs: 0, bytes: 0, done: false, failed: true })).toBe("Couldn't measure")
  })
})
