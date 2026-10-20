import { describe, expect, it } from 'vitest'
import type { ArchiveEntry } from '@/types'
import { archivePath, archiveStateFrom, archiveTree, searchArchive, withArchiveState } from '@/utils/archiveTree'

const file = (name: string, size: number): ArchiveEntry => ({ name, size, is_dir: false })
const dir = (name: string): ArchiveEntry => ({ name, size: 0, is_dir: true })

describe('archivePath', () => {
  it('drops the leading ./ and trailing / the formats write', () => {
    expect(archivePath('./WTF/Account/')).toBe('WTF/Account')
    expect(archivePath('/abs//double/a.txt')).toBe('abs/double/a.txt')
    expect(archivePath('./')).toBe('')
  })
})

describe('archiveTree', () => {
  it('lists each folder, folders first and numbers in order', () => {
    const { folders } = archiveTree([
      file('readme.txt', 1),
      dir('docs/'),
      file('docs/part10.txt', 1),
      file('docs/part2.txt', 1),
      dir('assets/'),
    ])
    expect(folders.get('')!.map((item) => item.name)).toEqual(['assets', 'docs', 'readme.txt'])
    expect(folders.get('docs')!.map((item) => item.name)).toEqual(['part2.txt', 'part10.txt'])
    expect(folders.get('assets')).toEqual([])
  })

  it('makes the folders an archive only implies', () => {
    const { folders } = archiveTree([file('./a/b/c.txt', 5)])
    expect(folders.get('')).toEqual([{ name: 'a', path: 'a', is_dir: true, size: 5 }])
    expect(folders.get('a')).toEqual([{ name: 'b', path: 'a/b', is_dir: true, size: 5 }])
    expect(folders.get('a/b')).toEqual([{ name: 'c.txt', path: 'a/b/c.txt', is_dir: false, size: 5 }])
  })

  it('totals the files under each folder', () => {
    const { folders } = archiveTree([file('a/one', 3), file('a/b/two', 4), dir('a/'), file('top', 100)])
    expect(folders.get('')!.find((item) => item.path === 'a')!.size).toBe(7)
    expect(folders.get('a')!.find((item) => item.path === 'a/b')!.size).toBe(4)
  })

  it('counts a file stored twice once, at its last size', () => {
    const { folders } = archiveTree([file('a/log', 10), file('a/log', 4)])
    expect(folders.get('a')).toEqual([{ name: 'log', path: 'a/log', is_dir: false, size: 4 }])
    expect(folders.get('')![0]!.size).toBe(4)
  })

  it('is empty for an empty archive', () => {
    expect(archiveTree([]).folders).toEqual(new Map([['', []]]))
    expect(archiveTree([dir('./')]).items).toEqual([])
  })
})

describe('searchArchive', () => {
  const { items } = archiveTree([
    file('WTF/Ann/SavedVariables/TimeManager.lua', 1),
    file('WTF/Bob/SavedVariables/TimeManager.lua', 1),
    file('Interface/AddOns/Timer/core.lua', 1),
  ])

  it('finds names under the folder at any depth, ignoring case', () => {
    expect(searchArchive(items, '', 'TIME').map((item) => item.path)).toEqual([
      'Interface/AddOns/Timer',
      'WTF/Ann/SavedVariables/TimeManager.lua',
      'WTF/Bob/SavedVariables/TimeManager.lua',
    ])
  })

  it('leaves out what is outside the folder, and the folder itself', () => {
    expect(searchArchive(items, 'WTF/Ann', 'time').map((item) => item.path)).toEqual(['WTF/Ann/SavedVariables/TimeManager.lua'])
    expect(searchArchive(items, 'WTF/Ann', 'ann')).toEqual([])
  })

  it('matches the name, not the folders above it', () => {
    expect(searchArchive(items, '', 'savedvariables').every((item) => item.is_dir)).toBe(true)
  })

  it('finds nothing for a blank query', () => {
    expect(searchArchive(items, '', '  ')).toEqual([])
  })
})

describe('archive state in the URL', () => {
  it('is closed without an archive', () => {
    expect(archiveStateFrom({ path: 'docs' })).toBeNull()
    expect(archiveStateFrom({ archive: '' })).toBeNull()
    expect(archiveStateFrom({ archive: null })).toBeNull()
  })

  it('reads the archive and the folder inside it, top by default', () => {
    expect(archiveStateFrom({ archive: 'old/Archive.tar.gz' })).toEqual({ path: 'old/Archive.tar.gz', at: '' })
    expect(archiveStateFrom({ archive: 'a.zip', archive_at: 'WTF/Ann/' })).toEqual({ path: 'a.zip', at: 'WTF/Ann' })
  })

  it('writes the state next to the browser\'s keys, leaving out the top', () => {
    const query = { path: 'old', q: 'tar', archive_at: 'stale' }
    expect(withArchiveState(query, { path: 'old/a.zip', at: '' })).toEqual({ path: 'old', q: 'tar', archive: 'old/a.zip' })
    expect(withArchiveState(query, { path: 'old/a.zip', at: 'WTF' })).toEqual({ path: 'old', q: 'tar', archive: 'old/a.zip', archive_at: 'WTF' })
  })

  it('closes, keeping the browser\'s keys', () => {
    expect(withArchiveState({ path: 'old', archive: 'old/a.zip', archive_at: 'WTF' }, null)).toEqual({ path: 'old' })
  })

  it('reads back what it writes', () => {
    const state = { path: 'a.zip', at: 'WTF/Ann' }
    expect(archiveStateFrom(withArchiveState({ path: 'x' }, state))).toEqual(state)
  })
})
