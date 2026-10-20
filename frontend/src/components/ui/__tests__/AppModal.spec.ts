// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
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

  it('fills the screen on phones', () => {
    const card = mountModal('lg')
    expect(card.classList).toContain('h-dvh')
    expect([...card.classList].filter((name) => name.startsWith('max-w'))).toEqual([])
  })
})

// Mounts a modal with these props over a button that opened it; returns its
// card and how many times it asked to close.
async function mountClosable(props: Record<string, unknown> = {}) {
  const closes: number[] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => [
      h('button', { id: 'opener' }, 'Open'),
      h(AppModal, { title: 'Settings', ...props, onClose: () => closes.push(1) }, () => h('input', { id: 'field' })),
    ],
  })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await nextTick()
  await nextTick()
  return { card: document.querySelector<HTMLElement>('[data-modal-card]')!, closes }
}

const closeButton = () => document.querySelector<HTMLButtonElement>('[data-modal-close]')!
const pressEscape = () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
const button = (label: string) => [...document.querySelectorAll<HTMLButtonElement>('button')].find((candidate) => candidate.textContent!.trim() === label)

describe('AppModal closing', () => {
  it('has the close button first in the header, before the title it is named by', async () => {
    const { card } = await mountClosable()
    const header = closeButton().parentElement!
    expect(header.firstElementChild).toBe(closeButton())
    expect(closeButton().getAttribute('aria-label')).toBe('Close')
    const title = header.querySelector('h2')!
    expect(title.textContent).toBe('Settings')
    expect(card.getAttribute('aria-labelledby')).toBe(title.id)
  })

  it('is named by its title when a slot shows something else', async () => {
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({ render: () => h(AppModal, { title: 'photos.zip' }, { title: () => h('nav', 'photos.zip › 2024') }) })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    expect(document.querySelector('[data-modal-card]')!.getAttribute('aria-label')).toBe('photos.zip')
  })

  it('closes on Escape unless told not to', async () => {
    const { closes } = await mountClosable()
    pressEscape()
    expect(closes).toHaveLength(1)
    cleanups.splice(0).forEach((cleanup) => cleanup())
    const kept = await mountClosable({ closeOnEscape: false })
    pressEscape()
    expect(kept.closes).toHaveLength(0)
  })

  it('closes from the button, and from a click outside, when nothing would be lost', async () => {
    const { card, closes } = await mountClosable()
    closeButton().click()
    card.parentElement!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    expect(closes).toHaveLength(2)
  })

  it('asks before losing changes, however it is closed', async () => {
    const { card, closes } = await mountClosable({ dirty: true })
    for (const tryClosing of [() => closeButton().click(), pressEscape, () => card.parentElement!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))]) {
      tryClosing()
      await nextTick()
      expect(document.body.textContent).toContain('Discard changes?')
      button('Keep editing')!.click()
      await vi.waitFor(() => expect(document.body.textContent).not.toContain('Discard changes?')) // after its fade
    }
    expect(closes).toHaveLength(0)

    closeButton().click()
    await nextTick()
    button('Discard')!.click()
    expect(closes).toHaveLength(1)
  })

  it('takes Escape for the question while it is asked', async () => {
    const { closes } = await mountClosable({ dirty: true })
    closeButton().click()
    await nextTick()
    pressEscape()
    await vi.waitFor(() => expect(document.body.textContent).not.toContain('Discard changes?'))
    expect(closes).toHaveLength(0)
  })
})
