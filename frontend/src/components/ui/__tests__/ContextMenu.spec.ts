// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import ContextMenu from '@/components/ui/ContextMenu.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

describe('ContextMenu', () => {
  it('closes on Escape and marks the key handled, so a preview or modal under it stays open', async () => {
    const closes: number[] = []
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => h(ContextMenu, { x: 0, y: 0, items: [{ label: 'Open' }], onClose: () => closes.push(1) }),
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    await nextTick()

    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    document.dispatchEvent(escape)
    expect(closes).toHaveLength(1)
    expect(escape.defaultPrevented).toBe(true)
  })
})
