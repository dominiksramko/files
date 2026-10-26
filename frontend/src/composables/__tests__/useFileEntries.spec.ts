import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import type { FileEntry } from '@/types'
import { useFileEntries } from '@/composables/useFileEntries'
import { files as filesApi, NotFoundError } from '@/utils/api'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop())
  vi.restoreAllMocks()
})

function entry(path: string): FileEntry {
  return { name: path.split('/').pop()!, path, is_dir: false, size: 1, mod_time: '2026-10-26T10:00:00Z', mime: 'image/png', has_thumb: false }
}

// The user's folder: folder → the paths in it.
function fakeList(folders: Record<string, string[]>) {
  return vi.spyOn(filesApi, 'list').mockImplementation(async (folder = '') => {
    const paths = folders[folder]
    if (!paths) throw new NotFoundError('Directory not found', '')
    return paths.map(entry)
  })
}

function watchEntries(paths: string[]) {
  const wanted = ref(paths)
  const scope = effectScope()
  scopes.push(scope)
  const entries = scope.run(() => useFileEntries(wanted))!
  return { wanted, ...entries }
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 20))

describe('useFileEntries', () => {
  it("lists each folder once and gives the files' details", async () => {
    const list = fakeList({ 'work/Roadmap files': ['work/Roadmap files/a.png', 'work/Roadmap files/b.pdf'], photos: ['photos/c.jpg'] })
    const { entryFor } = watchEntries(['work/Roadmap files/a.png', 'work/Roadmap files/b.pdf', 'photos/c.jpg'])
    expect(entryFor('photos/c.jpg')).toBeUndefined()

    await vi.waitFor(() => expect(entryFor('photos/c.jpg')).not.toBeUndefined())
    await vi.waitFor(() => expect(entryFor('work/Roadmap files/b.pdf')).not.toBeUndefined())
    expect(entryFor('work/Roadmap files/a.png')?.path).toBe('work/Roadmap files/a.png')
    expect(list.mock.calls.map(([folder]) => folder).sort()).toEqual(['photos', 'work/Roadmap files'])
  })

  it('reports a missing file, or a file in a missing folder, as missing', async () => {
    fakeList({ work: ['work/kept.txt'] })
    const { entryFor } = watchEntries(['work/gone.txt', 'nowhere/a.txt'])
    await vi.waitFor(() => expect(entryFor('work/gone.txt')).toBeNull())
    await vi.waitFor(() => expect(entryFor('nowhere/a.txt')).toBeNull())
  })

  it('lists a folder again for a file new to it (just uploaded), but not for one still missing', async () => {
    const folders: Record<string, string[]> = { work: ['work/a.txt'] }
    const list = fakeList(folders)
    const { wanted, entryFor } = watchEntries(['work/a.txt', 'work/gone.txt'])
    await vi.waitFor(() => expect(entryFor('work/a.txt')).not.toBeUndefined())
    expect(list).toHaveBeenCalledTimes(1)

    folders.work!.push('work/new.txt')
    wanted.value = [...wanted.value, 'work/new.txt']
    await vi.waitFor(() => expect(entryFor('work/new.txt')?.path).toBe('work/new.txt'))
    expect(list).toHaveBeenCalledTimes(2)

    wanted.value = [...wanted.value].reverse() // the same files in another order
    await nextTick()
    await settle()
    expect(list).toHaveBeenCalledTimes(2)
  })

  it('reads every folder again on refresh', async () => {
    const folders: Record<string, string[]> = { work: ['work/a.txt'] }
    fakeList(folders)
    const { entryFor, refresh } = watchEntries(['work/a.txt'])
    await vi.waitFor(() => expect(entryFor('work/a.txt')).not.toBeNull())

    folders.work = []
    refresh()
    await vi.waitFor(() => expect(entryFor('work/a.txt')).toBeNull())
  })
})
