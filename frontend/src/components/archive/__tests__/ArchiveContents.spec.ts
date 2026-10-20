// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import type { ArchiveEntry } from '@/types'
import { files as filesApi, PasswordRequiredError } from '@/utils/api'
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
const entries = (encrypted = false): ArchiveEntry[] => names.map((name) => ({ name, size: 1, is_dir: false, encrypted }))

// Mounts the modal as the file browser does, from the URL, over an archive
// listed by list (given the password typed, '' at first); returns the router.
async function mountArchive(url: string, list: (password: string) => Promise<ArchiveEntry[]> = async () => entries()): Promise<Router> {
  const contents = vi.spyOn(filesApi, 'archiveContents').mockImplementation((_path, password) => list(password ?? ''))
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
  await new Promise((resolve) => setTimeout(resolve)) // the listing shown
  return router
}

const rows = () => [...document.querySelectorAll<HTMLElement>('[data-archive-item]')]
const rowNames = () => rows().map((row) => row.querySelector('.truncate')!.textContent!.trim())
const row = (name: string) => rows().find((candidate) => candidate.querySelector('.truncate')!.textContent!.trim() === name)!
const card = () => document.querySelector<HTMLElement>('[role="dialog"]')!
const button = (label: string) => [...document.querySelectorAll<HTMLButtonElement>('button')].find((candidate) => candidate.textContent!.trim() === label)!
const inputs = () => [...document.querySelectorAll<HTMLInputElement>('input')]
// The password field shown last (the dialog's is on top of the modal's).
const passwordField = () => [...document.querySelectorAll<HTMLInputElement>('input[placeholder="Enter password"]')].slice(-1)[0]!
const topDialog = () => [...document.querySelectorAll('[aria-modal="true"]')].slice(-1)[0]!

async function type(input: HTMLInputElement, text: string) {
  input.value = text
  input.dispatchEvent(new Event('input'))
  await nextTick()
}
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
    await mountArchive('/?archive=locked.7z', () => Promise.reject(new PasswordRequiredError()))
    await vi.waitFor(() => expect(document.body.textContent).toContain('file names are password protected'))
    expect(card().classList).not.toContain('h-[90vh]')
    cleanups.splice(0).forEach((cleanup) => cleanup())
    vi.restoreAllMocks()
    await mountArchive('/?archive=broken.zip', () => Promise.reject(new Error('not a valid zip file')))
    await vi.waitFor(() => expect(document.body.textContent).toContain('not a valid zip file'))
    expect(card().classList).not.toContain('h-[90vh]')
  })

  it('asks for no password when the archive needs none', async () => {
    const extract = vi.spyOn(filesApi, 'extract').mockResolvedValue({ message: 'extracted' })
    const router = await mountArchive('/?path=old&archive=old/Archive.tar.gz')
    expect(inputs()).toHaveLength(1) // the search; no password field
    expect(document.querySelector('[data-archive-lock], [data-archive-encrypted]')).toBeNull()
    button('Extract here').click()
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({ path: 'old' }))
    expect(extract.mock.calls).toEqual([['old/Archive.tar.gz', undefined, undefined, 'error']])
  })

  it('marks encrypted files, and asks for the password on top of the modal before extracting', async () => {
    const extract = vi.spyOn(filesApi, 'extract').mockImplementation(async (_path, _dest, password) => {
      if (password !== 'hunter2') throw new PasswordRequiredError()
      return { message: 'extracted' }
    })
    const router = await mountArchive('/?archive=locked.zip', async () => entries(true))
    expect(row('readme.txt').querySelector('[data-archive-lock]')).not.toBeNull()
    expect(row('WTF').querySelector('[data-archive-lock]')).toBeNull()
    expect(document.querySelector('[data-archive-encrypted]')).not.toBeNull()

    button('Extract here').click()
    await vi.waitFor(() => expect(document.body.textContent).toContain('Password required'))
    expect(extract).not.toHaveBeenCalled()
    expect(topDialog().textContent).toContain('Password required')

    const passwordInput = passwordField()
    expect(topDialog().contains(passwordInput)).toBe(true)
    await type(passwordInput, 'nope')
    passwordInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await vi.waitFor(() => expect(topDialog().textContent).toContain('Wrong password'))
    await type(passwordInput, 'hunter2')
    passwordInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    await vi.waitFor(() => expect(router.currentRoute.value.query).toEqual({}))
    expect(extract.mock.calls.map((call) => call[2])).toEqual(['nope', 'hunter2'])
  })

  it('unlocks encrypted names, then extracts with that password without asking', async () => {
    const extract = vi.spyOn(filesApi, 'extract').mockResolvedValue({ message: 'extracted' })
    await mountArchive('/?archive=locked.7z', async (password) => {
      if (password !== 'hunter2') throw new PasswordRequiredError()
      return entries(true)
    })
    const passwordInput = passwordField()
    await type(passwordInput, 'nope')
    button('Unlock').click()
    await vi.waitFor(() => expect(document.body.textContent).toContain('Wrong password'))
    await type(passwordInput, 'hunter2')
    button('Unlock').click()
    await shows(['WTF', 'readme.txt'])

    button('Extract here').click()
    await vi.waitFor(() => expect(extract).toHaveBeenCalled())
    expect(document.body.textContent).not.toContain('Password required')
    expect(extract.mock.calls[0]![2]).toBe('hunter2')
  })

  it('names the archive once, as the first crumb of the breadcrumb in the header', async () => {
    await mountArchive('/?archive=old/Archive.tar.gz&archive_at=WTF')
    expect(card().textContent!.split('Archive.tar.gz')).toHaveLength(2)
    expect(crumb('Archive.tar.gz')).toBeTruthy()
    expect(crumb('WTF')).toBeTruthy()
  })

  it('says it is reading the archive while the listing loads', async () => {
    let finish: (listing: ArchiveEntry[]) => void = () => {}
    await mountArchive('/?archive=Archive.tar.gz', () => new Promise((resolve) => { finish = resolve }))
    expect(document.querySelector('[role="status"]')!.textContent).toContain('Reading the archive…')
    finish(entries())
    await shows(['WTF', 'readme.txt'])
    expect(document.querySelector('[role="status"]')).toBeNull()
  })
})
