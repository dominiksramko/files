// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, nextTick, ref } from 'vue'
import { useEscapeToClose } from '@/composables/useEscapeToClose'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

// A dialog card in the page (inside parent) that counts how often it's
// closed; open says whether it's open.
function mountDialog(open = ref(true), parent: Element = document.body) {
  const card = document.createElement('div')
  card.setAttribute('aria-modal', 'true')
  parent.append(card)
  const closed = ref(0)
  const app = createApp(defineComponent({
    setup() {
      useEscapeToClose(ref(card), () => open.value, () => { closed.value++ })
      return () => null
    },
  }))
  app.mount(document.createElement('div'))
  cleanups.push(() => { app.unmount(); card.remove() })
  return { card, closed, unmount: () => app.unmount() }
}

function pressEscape(prevented = false): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true })
  if (prevented) event.preventDefault()
  window.dispatchEvent(event)
  return event
}

describe('useEscapeToClose', () => {
  it('closes the open dialog and keeps the key from others', () => {
    const { closed } = mountDialog()
    const event = pressEscape()
    expect(closed.value).toBe(1)
    expect(event.defaultPrevented).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    expect(closed.value).toBe(1)
  })

  it('closes only the dialog on top', () => {
    const below = mountDialog()
    const above = mountDialog(ref(true), below.card)
    pressEscape()
    expect(above.closed.value).toBe(1)
    expect(below.closed.value).toBe(0)
  })

  it('leaves a key an input already handled alone', () => {
    const { closed } = mountDialog()
    pressEscape(true)
    expect(closed.value).toBe(0)
  })

  it('listens only while open and mounted', async () => {
    const open = ref(false)
    const { closed, unmount } = mountDialog(open)
    pressEscape()
    expect(closed.value).toBe(0)
    open.value = true
    await nextTick()
    pressEscape()
    expect(closed.value).toBe(1)
    unmount()
    pressEscape()
    expect(closed.value).toBe(1)
  })
})
