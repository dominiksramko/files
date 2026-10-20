// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import AppLoading from '@/components/ui/AppLoading.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

function mountLoading(label: string, extra?: string) {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({ render: () => h(AppLoading, { label }, extra ? () => h('p', extra) : undefined) })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  return root.querySelector<HTMLElement>('[role="status"]')!
}

describe('AppLoading', () => {
  it('says what is loading, next to a spinner, as a status', () => {
    const status = mountLoading('Reading the archive…')
    expect(status.textContent).toContain('Reading the archive…')
    expect(status.querySelector('svg[aria-label="Loading"]')).not.toBeNull()
  })

  it('shows more below the label', () => {
    expect(mountLoading('Scanning…', '12 files').textContent).toContain('12 files')
  })
})
