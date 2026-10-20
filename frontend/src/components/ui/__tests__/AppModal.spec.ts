// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import AppModal from '@/components/ui/AppModal.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

// Mounts a modal of the given size; returns its card.
function mountModal(size?: 'sm' | 'md' | 'lg' | 'xl' | '2xl') {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({ render: () => h(AppModal, { title: 'Title', size }) })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  return document.querySelector<HTMLElement>('[role="dialog"]')!
}

describe('AppModal', () => {
  it('is medium width unless given a size', () => {
    expect(mountModal().className).toContain('max-w-lg')
  })

  it('keeps the small sizes the same on every screen', () => {
    expect(mountModal('sm').className).not.toMatch(/\bx?l:|2xl:/)
  })

  it('lets the larger sizes grow on big screens', () => {
    expect(mountModal('lg').className).toContain('xl:max-w-4xl')
    cleanups.splice(0).forEach((cleanup) => cleanup())
    expect(mountModal('xl').className).toContain('2xl:max-w-6xl')
  })

  it('shows the title slot in place of the title, with the close button', () => {
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({ render: () => h(AppModal, { title: 'Plain title' }, { title: () => h('nav', 'a › b') }) })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    const card = document.querySelector<HTMLElement>('[role="dialog"]')!
    expect(card.querySelector('nav')!.textContent).toBe('a › b')
    expect(card.textContent).not.toContain('Plain title')
    expect(card.querySelector('button[aria-label="Close"]')).not.toBeNull()
  })
})
