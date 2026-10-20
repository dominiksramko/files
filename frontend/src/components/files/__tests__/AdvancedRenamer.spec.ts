// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { FileEntry } from '@/types'
import { renamer } from '@/utils/api'
import AdvancedRenamer from '@/components/files/AdvancedRenamer.vue'

const cleanups: (() => void)[] = []
beforeEach(() => {
  vi.spyOn(renamer, 'listPresets').mockResolvedValue([])
  vi.spyOn(renamer, 'preview').mockResolvedValue({ items: [] })
})
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.restoreAllMocks()
})

const file: FileEntry = {
  name: 'a.txt', path: 'docs/a.txt', is_dir: false, size: 1, mod_time: '2026-10-21T10:00:00Z', mime: 'text/plain', has_thumb: false,
}

// Mounts the renamer in a folder whose saved template is savedTemplate;
// returns how often it closed.
async function mountRenamer(savedTemplate = '') {
  const folderTemplate = vi.spyOn(renamer, 'getFolderTemplate').mockResolvedValue({ template: savedTemplate })
  const closes: number[] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({ render: () => h(AdvancedRenamer, { files: [file], folder: 'docs', onClose: () => closes.push(1) }) })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await vi.waitFor(() => expect(folderTemplate).toHaveBeenCalled())
  await new Promise((resolve) => setTimeout(resolve))
  return closes
}

const pressEscape = () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
const button = (label: string) => [...document.querySelectorAll<HTMLButtonElement>('button')].find((candidate) => candidate.textContent!.includes(label))!
const popovers = () => document.querySelectorAll('.fixed.z-\\[61\\]')

describe('AdvancedRenamer', () => {
  it('closes an open popover on Escape, and only then the modal', async () => {
    const closes = await mountRenamer()
    button('Variables').click()
    await nextTick()
    expect(popovers()).toHaveLength(1)

    pressEscape()
    await nextTick()
    expect(popovers()).toHaveLength(0)
    expect(closes).toHaveLength(0)

    pressEscape()
    expect(closes).toHaveLength(1)
  })

  it("doesn't count the folder's saved template as a change", async () => {
    const closes = await mountRenamer('{name} copy.{ext}')
    pressEscape()
    expect(closes).toHaveLength(1)
    expect(document.body.textContent).not.toContain('Discard changes?')
  })
})
