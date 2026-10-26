import { describe, expect, it } from 'vitest'
import { breadcrumbTrail, isWithin, nameOf, parentOf, relativePath, relativeTo, resolvePath } from '@/utils/paths'

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

describe('resolvePath', () => {
  it('follows a link from a folder', () => {
    expect(resolvePath('work', 'Roadmap files/mockup.png')).toBe('work/Roadmap files/mockup.png')
    expect(resolvePath('work/boards', '../specs/login.pdf')).toBe('work/specs/login.pdf')
    expect(resolvePath('work', './a//b.txt')).toBe('work/a/b.txt')
    expect(resolvePath('', 'a.txt')).toBe('a.txt')
  })

  it('starts from the home folder for a link starting with /', () => {
    expect(resolvePath('work/boards', '/photos/a.jpg')).toBe('photos/a.jpg')
  })

  it('reaches nothing above the home folder', () => {
    expect(resolvePath('work', '../../etc/passwd')).toBeNull()
    expect(resolvePath('', '../a.txt')).toBeNull()
    expect(resolvePath('work', '/../a.txt')).toBeNull()
  })

  it('keeps names with spaces, % and # as they are', () => {
    expect(resolvePath('', 'my files/50% off #2.pdf')).toBe('my files/50% off #2.pdf')
  })
})

describe('relativePath', () => {
  it('links a file from a folder', () => {
    expect(relativePath('work', 'work/Roadmap files/mockup.png')).toBe('Roadmap files/mockup.png')
    expect(relativePath('work/boards', 'work/specs/login.pdf')).toBe('../specs/login.pdf')
    expect(relativePath('work', 'photos/a.jpg')).toBe('../photos/a.jpg')
    expect(relativePath('', 'photos/a.jpg')).toBe('photos/a.jpg')
    expect(relativePath('work', 'notes.txt')).toBe('../notes.txt')
  })

  it("doesn't take a folder sharing a name's start for the same folder", () => {
    expect(relativePath('work', 'workshop/a.txt')).toBe('../workshop/a.txt')
  })

  it('makes links resolvePath turns back into the path', () => {
    const cases: [string, string][] = [['a/b', 'a/c/d.txt'], ['a', 'a/a/a'], ['', 'x'], ['a/b/c', 'd.txt'], ['a', 'a.txt']]
    for (const [folder, path] of cases) expect(resolvePath(folder, relativePath(folder, path))).toBe(path)
  })
})
