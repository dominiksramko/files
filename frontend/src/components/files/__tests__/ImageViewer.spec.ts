// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { FileEntry } from '@/types'
import ImageViewer from '@/components/files/ImageViewer.vue'

const cleanups: (() => void)[] = []
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.unstubAllGlobals()
})

const png: FileEntry = {
  name: 'shot.png', path: 'shots/shot.png', is_dir: false, size: 3, mod_time: '2026-10-21T10:00:00Z',
  mime: 'image/png', has_thumb: true, thumb_hash: 'h1',
}

async function mountViewer(entry: FileEntry) {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({ render: () => h(ImageViewer, { entry }) })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await nextTick()
  const image = root.querySelector<HTMLImageElement>('img[alt="shot.png"]')!
  const placeholder = () => root.querySelector<HTMLImageElement>('img[aria-hidden="true"]')
  return { root, image, placeholder }
}

describe('ImageViewer', () => {
  it('shows the preview-sized thumbnail until the image has loaded', async () => {
    const { image, placeholder } = await mountViewer(png)
    expect(placeholder()?.getAttribute('src')).toBe('/api/v1/thumbnails/preview?path=shots%2Fshot.png&hash=h1')

    image.dispatchEvent(new Event('load'))
    await nextTick()
    expect(placeholder()).toBeNull()
  })

  it('puts the checkerboard behind a transparent image only once it has loaded, so the placeholder shows meanwhile', async () => {
    const { image } = await mountViewer(png)
    expect(image.classList.contains('checkerboard')).toBe(false)

    image.dispatchEvent(new Event('load'))
    await nextTick()
    expect(image.classList.contains('checkerboard')).toBe(true)
  })

  it('puts the loaded image in place without sliding it there', async () => {
    const { image } = await mountViewer(png)
    Object.defineProperty(image, 'naturalWidth', { value: 800 })
    Object.defineProperty(image, 'naturalHeight', { value: 600 })
    // The transition in force each time the image's style changes.
    const placements: { left: string; transition: string }[] = []
    const observer = new MutationObserver(() => placements.push({ left: image.style.left, transition: image.style.transition }))
    observer.observe(image, { attributes: true, attributeFilter: ['style'] })
    cleanups.push(() => observer.disconnect())

    image.dispatchEvent(new Event('load'))
    await nextTick()
    await nextTick()
    await nextTick()
    const firstPlaced = placements.find((placement) => placement.left === '0px')
    expect(firstPlaced?.transition).toBe('none')
    // Zooming still animates.
    expect(image.style.transition).toBe('transform 0.1s ease')
  })

  it('shows no placeholder without a thumbnail, or when it fails to load', async () => {
    const { placeholder: withoutThumb } = await mountViewer({ ...png, has_thumb: false, thumb_hash: undefined })
    expect(withoutThumb()).toBeNull()

    const { placeholder } = await mountViewer(png)
    placeholder()!.dispatchEvent(new Event('error'))
    await nextTick()
    expect(placeholder()).toBeNull()
  })
})
