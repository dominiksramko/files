import { describe, expect, it } from 'vitest'
import { breadcrumbTrail, isWithin, nameOf, parentOf, relativeTo } from '@/utils/paths'

describe('paths', () => {
  it('finds the name a path leads to', () => {
    expect(nameOf('a/b/c.txt')).toBe('c.txt')
    expect(nameOf('c.txt')).toBe('c.txt')
    expect(nameOf('')).toBe('')
  })

  it('finds the parent folder', () => {
    expect(parentOf('a/b/c.txt')).toBe('a/b')
    expect(parentOf('c.txt')).toBe('')
  })

  it('knows what is inside a folder', () => {
    expect(isWithin('', 'anything/at/all')).toBe(true)
    expect(isWithin('media', 'media')).toBe(true)
    expect(isWithin('media', 'media/clips')).toBe(true)
    expect(isWithin('media', 'media2/clips')).toBe(false)
    expect(isWithin('media', '')).toBe(false)
  })

  it('shows paths from a folder', () => {
    expect(relativeTo('media', 'media/clips/one.bin')).toBe('clips/one.bin')
    expect(relativeTo('', 'media/one.bin')).toBe('media/one.bin')
    expect(relativeTo('media', 'media')).toBe('')
  })
})

describe('breadcrumbTrail', () => {
  it('starts at Home', () => {
    expect(breadcrumbTrail('')).toEqual([{ label: 'Home', path: '', isHome: true }])
    expect(breadcrumbTrail('a/b')).toEqual([
      { label: 'Home', path: '', isHome: true },
      { label: 'a', path: 'a', isHome: false },
      { label: 'b', path: 'a/b', isHome: false },
    ])
  })

  it('starts at a given folder, showing nothing above it', () => {
    expect(breadcrumbTrail('media/clips/2024', 'media/clips')).toEqual([
      { label: 'clips', path: 'media/clips', isHome: true },
      { label: '2024', path: 'media/clips/2024', isHome: false },
    ])
    expect(breadcrumbTrail('media/clips', 'media/clips')).toHaveLength(1)
    expect(breadcrumbTrail('other', 'media')).toEqual([{ label: 'media', path: 'media', isHome: true }])
  })

  it('labels the first crumb as asked', () => {
    expect(breadcrumbTrail('docs', '', 'photos.zip')).toEqual([
      { label: 'photos.zip', path: '', isHome: true },
      { label: 'docs', path: 'docs', isHome: false },
    ])
  })
})
