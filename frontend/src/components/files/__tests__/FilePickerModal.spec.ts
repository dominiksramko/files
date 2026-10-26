// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { FileEntry } from '@/types'
import FilePickerModal from '@/components/files/FilePickerModal.vue'
import { files as filesApi, NotFoundError } from '@/utils/api'

const cleanups: (() => void)[] = []
beforeEach(() => {
  // The file chooser's breadcrumb measures itself; jsdom has no layout.
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function entry(path: string, isDir = false): FileEntry {
  return { name: path.split('/').pop()!, path, is_dir: isDir, size: 10, mod_time: '', mime: isDir ? '' : 'text/plain', has_thumb: false }
}

// The user's folder: folder → what's in it.
const tree: Record<string, FileEntry[]> = {
  work: [entry('work/b.txt'), entry('work/specs', true), entry('work/.hidden'), entry('work/a10.txt'), entry('work/a2.txt')],
  'work/specs': [entry('work/specs/login.pdf')],
}

async function mountPicker(folder = 'work') {
  vi.spyOn(filesApi, 'list').mockImplementation(async (path = '') => {
    if (tree[path]) return tree[path]
    throw new NotFoundError('Directory not found', 'work')
  })
  const events: unknown[][] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => h(FilePickerModal, {
      title: 'Attach files', folder, chooseLabel: 'Attach',
      onClose: () => events.push(['close']),
      onChoose: (paths: string[]) => events.push(['choose', paths]),
    }),
  })
  app.use(createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }] }))
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await shown()
  return events
}

const shown = () => vi.waitFor(() => expect(document.querySelector('[data-picker-entries]')).not.toBeNull())
const rows = () => [...document.querySelectorAll<HTMLElement>('[data-picker-entries] li')]
const rowNamed = (name: string) => rows().find((row) => row.textContent?.includes(name))!.firstElementChild as HTMLElement
const button = (label: string) => [...document.querySelectorAll('button')].find((candidate) => candidate.textContent?.trim() === label)!

describe('FilePickerModal', () => {
  it('lists the folder, folders first, names in order, hidden files left out', async () => {
    await mountPicker()
    expect(rows().map((row) => row.textContent?.replace(/\s*10 B$/, '').trim())).toEqual(['specs', 'a2.txt', 'a10.txt', 'b.txt'])
  })

  it('chooses the files ticked, in several folders', async () => {
    const events = await mountPicker()
    expect((button('Attach') as HTMLButtonElement).disabled).toBe(true)
    rowNamed('b.txt').click()
    rowNamed('specs').click()
    await vi.waitFor(() => expect(rows().map((row) => row.textContent?.trim())).toEqual(['login.pdf10 B']))
    rowNamed('login.pdf').click()
    await nextTick()
    expect(document.body.textContent).toContain('2 files selected')
    button('Attach').click()
    expect(events).toEqual([['choose', ['work/b.txt', 'work/specs/login.pdf']]])
  })

  it('unticks a file clicked again', async () => {
    const events = await mountPicker()
    rowNamed('b.txt').click()
    await nextTick()
    rowNamed('b.txt').click()
    await nextTick()
    expect((button('Attach') as HTMLButtonElement).disabled).toBe(true)
    expect(events).toEqual([])
  })

  it('chooses just the file double-clicked', async () => {
    const events = await mountPicker()
    rowNamed('a2.txt').dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
    expect(events).toEqual([['choose', ['work/a2.txt']]])
  })

  it('starts in the nearest folder still there when the one asked for is gone', async () => {
    await mountPicker('work/gone')
    await vi.waitFor(() => expect(rows()).toHaveLength(4))
    expect(filesApi.list).toHaveBeenLastCalledWith('work')
  })

  it('closes without choosing on Cancel', async () => {
    const events = await mountPicker()
    rowNamed('b.txt').click()
    button('Cancel').click()
    expect(events).toEqual([['close']])
  })
})
