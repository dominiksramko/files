// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { files as filesApi } from '@/utils/api'
import { archiveStateFrom } from '@/utils/archiveTree'
import ArchiveContents from '@/components/archive/ArchiveContents.vue'

const cleanups: (() => void)[] = []
beforeEach(() => {
  // The breadcrumb measures itself; jsdom has no layout.
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const names = ['./WTF/Ann/SavedVariables/TimeManager.lua', './WTF/Bob/SavedVariables/TimeManager.lua', './readme.txt']

// Mounts the modal as the file browser does, from the URL, over an archive
// holding names (or one that can't be read: failure); returns the router.
async function mountArchive(url: string, failure?: Error): Promise<Router> {
  const contents = vi.spyOn(filesApi, 'archiveContents')
  if (failure) contents.mockRejectedValue(failure)
  else contents.mockResolvedValue(names.map((name) => ({ name, size: 1, is_dir: false })))
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }] })
  await router.push(url)
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => {
      const state = archiveStateFrom(router.currentRoute.value.query)
      return state ? h(ArchiveContents, { state }) : null
    },
  })
  app.use(router)
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await vi.waitFor(() => expect(contents).toHaveBeenCalled())
  if (!failure) await vi.waitFor(() => expect(rows().length).toBeGreaterThan(0))
  return router
}

const rows = () => [...document.querySelectorAll<HTMLElement>('[data-archive-item]')]
const rowNames = () => rows().map((row) => row.querySelector('.truncate')!.textContent!.trim())
const row = (name: string) => rows().find((candidate) => candidate.querySelector('.truncate')!.textContent!.trim() === name)!
const card = () => document.querySelector<HTMLElement>('[role="dialog"]')!
const crumb = (label: string) => [...document.querySelectorAll<HTMLAnchorElement>('nav a')].find((link) => link.textContent!.trim() === label)!

async function shows(expected: string[]) {
  await vi.waitFor(() => expect(rowNames()).toEqual(expected))
}

async function search(query: string) {
  const input = document.querySelector<HTMLInputElement>('input[type="text"]')!
  input.value = query
  input.dispatchEvent(new Event('input'))
  await nextTick()
}

describe('ArchiveContents', () => {
  it('shows the top of the archive, and puts a folder opened in the URL', async () => {
    const router = await mountArchive('/?path=old&archive=old/Archive.tar.gz')
    expect(rowNames()).toEqual(['WTF', 'readme.txt'])
    row('WTF').click()
    await shows(['Ann', 'Bob'])
    expect(router.currentRoute.value.query).toEqual({ path: 'old', archive: 'old/Archive.tar.gz', archive_at: 'WTF' })
  })

  it('opens at the folder in the URL, so a refresh stays there', async () => {
    await mountArchive('/?archive=Archive.tar.gz&archive_at=WTF/Ann')
    expect(rowNames()).toEqual(['SavedVariables'])
    expect(crumb('Ann')).toBeTruthy()
  })

  it('shows the top for a folder the archive does not have', async () => {
    await mountArchive('/?archive=Archive.tar.gz&archive_at=nowhere')
    expect(rowNames()).toEqual(['WTF', 'readme.txt'])
  })

  it('goes back up with Back and through the breadcrumb', async () => {
    const router = await mountArchive('/?archive=Archive.tar.gz')
    row('WTF').click()
    await shows(['Ann', 'Bob'])
    row('Ann').click()
    await shows(['SavedVariables'])
    router.back()
    await shows(['Ann', 'Bob'])
    expect(crumb('Archive.tar.gz').getAttribute('href')).toBe('/?archive=Archive.tar.gz')
    crumb('Archive.tar.gz').click()
    await shows(['WTF', 'readme.txt'])
  })

  it('searches the folder and below, and opens the folder holding a result', async () => {
    const router = await mountArchive('/?archive=Archive.tar.gz&archive_at=WTF')
    await search('timemanager')
    const parts = (result: HTMLElement) => [...result.querySelectorAll('span')].map((part) => part.textContent!.trim()).filter(Boolean).slice(1)
    expect(rows().map(parts)).toEqual([
      ['TimeManager.lua', 'in Ann/SavedVariables', '1 B'],
      ['TimeManager.lua', 'in Bob/SavedVariables', '1 B'],
    ])
    rows()[1]!.click()
    await shows(['TimeManager.lua'])
    expect(router.currentRoute.value.query.archive_at).toBe('WTF/Bob/SavedVariables')
    expect(document.querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('')
  })

  it('does nothing when a file is clicked outside a search', async () => {
    const router = await mountArchive('/?archive=Archive.tar.gz')
    row('readme.txt').click()
    await nextTick()
    expect(rowNames()).toEqual(['WTF', 'readme.txt'])
    expect(router.currentRoute.value.query).toEqual({ archive: 'Archive.tar.gz' })
  })

  it('closes by leaving the URL, keeping the browser\'s keys', async () => {
    const router = await mountArchive('/?path=old&archive=old/Archive.tar.gz&archive_at=WTF')
    document.querySelector<HTMLButtonElement>('button[aria-label="Close"]')!.click()
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ path: 'old' }))
    expect(rows()).toEqual([])
  })

  it('keeps the same height in every folder', async () => {
    await mountArchive('/?archive=Archive.tar.gz')
    expect(card().classList).toContain('h-[90vh]')
    row('WTF').click()
    await shows(['Ann', 'Bob'])
    expect(card().classList).toContain('h-[90vh]')
  })

  it('fits a password prompt or an error to its size', async () => {
    await mountArchive('/?archive=locked.7z', new Error('archive is password protected'))
    await vi.waitFor(() => expect(document.body.textContent).toContain('This archive is password protected'))
    expect(card().classList).not.toContain('h-[90vh]')
    cleanups.splice(0).forEach((cleanup) => cleanup())
    vi.restoreAllMocks()
    await mountArchive('/?archive=broken.zip', new Error('not a valid zip file'))
    await vi.waitFor(() => expect(document.body.textContent).toContain('not a valid zip file'))
    expect(card().classList).not.toContain('h-[90vh]')
  })
})
