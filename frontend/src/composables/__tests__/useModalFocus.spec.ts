// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, h, nextTick, ref, type Ref } from 'vue'
import { focusableIn, useModalFocus } from '@/composables/useModalFocus'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

// A modal card holding children, open while open is true.
const TestModal = defineComponent({
  props: { open: { type: Boolean, required: true } },
  setup(props, { slots }) {
    const card = ref<HTMLElement | null>(null)
    useModalFocus(card, () => props.open)
    return () => (props.open ? h('div', { ref: card, role: 'dialog', 'aria-modal': 'true', tabindex: '-1' }, slots.default?.()) : null)
  },
})

// Mounts an opener button and the modal (children from content); returns
// the open flag and the opener.
async function mountModal(content: () => unknown, open: Ref<boolean> = ref(false)) {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => [h('button', { id: 'opener' }, 'Open'), h(TestModal, { open: open.value }, content)],
  })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  const opener = document.getElementById('opener')!
  opener.focus()
  open.value = true
  await nextTick()
  await nextTick()
  return { open, opener }
}

const focused = () => document.activeElement as HTMLElement
const card = () => document.querySelector<HTMLElement>('[role="dialog"]')!

function pressTab(shift = false) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: shift, cancelable: true }))
}

describe('useModalFocus', () => {
  it('moves focus to the autofocus element when it opens', async () => {
    await mountModal(() => [h('button', 'Cancel'), h('input', { id: 'name', autofocus: true })])
    expect(focused().id).toBe('name')
  })

  it('moves focus to the card without one', async () => {
    await mountModal(() => [h('button', 'Cancel')])
    expect(focused()).toBe(card())
  })

  it('keeps Tab going round inside', async () => {
    await mountModal(() => [h('button', { id: 'first' }, 'A'), h('button', { id: 'last' }, 'B')])
    document.getElementById('last')!.focus()
    pressTab()
    expect(focused().id).toBe('first')
    pressTab(true)
    expect(focused().id).toBe('last')
  })

  it('brings focus back in from outside, and Shift+Tab from the card goes to the end', async () => {
    await mountModal(() => [h('button', { id: 'first' }, 'A'), h('button', { id: 'last' }, 'B')])
    pressTab(true)
    expect(focused().id).toBe('last')
    document.getElementById('opener')!.focus()
    pressTab()
    expect(focused().id).toBe('first')
  })

  it('gives focus back to the opener when it closes', async () => {
    const { open, opener } = await mountModal(() => [h('button', 'Cancel')])
    expect(focused()).toBe(card())
    open.value = false
    await nextTick()
    await nextTick()
    expect(focused()).toBe(opener)
  })

  it('leaves alone a dialog that is not on top', async () => {
    await mountModal(() => [
      h('button', { id: 'behind' }, 'A'),
      h('div', { role: 'dialog', 'aria-modal': 'true' }, [h('button', { id: 'above' }, 'B')]),
    ])
    // The inner dialog is on top: the outer card's Tab handling stays out of it.
    document.getElementById('above')!.focus()
    pressTab()
    expect(focused().id).toBe('above')
  })
})

describe('focusableIn', () => {
  it('lists what Tab reaches, without the disabled or hidden', () => {
    const container = document.createElement('div')
    container.innerHTML = `
      <button id="one">1</button>
      <button disabled>no</button>
      <div style="display: none"><button>hidden</button></div>
      <a href="#" id="two">2</a>
      <a>no href</a>
      <input id="three">
      <span tabindex="-1">no</span>
      <span tabindex="0" id="four">4</span>`
    expect(focusableIn(container).map((element) => element.id)).toEqual(['one', 'two', 'three', 'four'])
  })
})
