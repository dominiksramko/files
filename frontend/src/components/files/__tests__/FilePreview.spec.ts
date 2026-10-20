// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { FileEntry } from '@/types'
import FilePreview from '@/components/files/FilePreview.vue'

// jsdom has no media queries, and the video player (plyr) asks one as it's
// imported: before the imports.
vi.hoisted(() => {
  window.matchMedia = (query: string) =>
    ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }) as unknown as MediaQueryList
})

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

// A file with no viewer of its own, so the preview shows its fallback.
const entry: FileEntry = {
  name: 'data.bin', path: 'data.bin', is_dir: false, size: 3, mod_time: '2026-10-21T10:00:00Z', mime: 'application/octet-stream', has_thumb: false,
}

describe('FilePreview', () => {
  it('has the close button first in its top bar, on the left as in the modals', async () => {
    const closes: number[] = []
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => h(FilePreview, { entry, hasPrev: false, hasNext: false, index: 0, total: 1, onClose: () => closes.push(1) }),
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    await nextTick()

    const close = document.querySelector<HTMLButtonElement>('button[aria-label="Close"]')!
    const topBar = close.parentElement!
    expect(topBar.firstElementChild).toBe(close)
    expect(topBar.textContent).toContain('data.bin')
    close.click()
    expect(closes).toHaveLength(1)
  })
})
