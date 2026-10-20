// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, h } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useHeaderCenter } from '@/composables/useHeaderCenter'
import { useLayoutStore } from '@/stores/layout'

// Mounts a view that puts text in the header; returns a function unmounting it.
function mountView(text: string): () => void {
  const view = defineComponent({
    setup() {
      useHeaderCenter('Test', () => h('span', text))
      return () => null
    },
  })
  const app = createApp(view)
  app.mount(document.createElement('div'))
  return () => app.unmount()
}

beforeEach(() => setActivePinia(createPinia()))

describe('useHeaderCenter', () => {
  it('fills the header while the view is mounted', () => {
    const layout = useLayoutStore()
    const unmount = mountView('crumbs')
    expect(layout.headerBreadcrumbsActive).toBe(true)
    expect(layout.headerCenter).not.toBeNull()
    unmount()
    expect(layout.headerBreadcrumbsActive).toBe(false)
    expect(layout.headerCenter).toBeNull()
  })

  it('leaves the next view\'s header alone when unmounted late', () => {
    const layout = useLayoutStore()
    const unmountFirst = mountView('first')
    const unmountSecond = mountView('second')
    const second = layout.headerCenter
    unmountFirst()
    expect(layout.headerCenter).toBe(second)
    expect(layout.headerBreadcrumbsActive).toBe(true)
    unmountSecond()
  })
})
