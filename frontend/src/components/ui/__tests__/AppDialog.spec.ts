// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import AppDialog from '@/components/ui/AppDialog.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

function mountDialog(slots: Record<string, () => unknown>) {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({ render: () => h(AppDialog, { open: true }, slots) })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  return document.querySelector<HTMLElement>('[role="dialog"]')!
}

describe('AppDialog', () => {
  it('puts the actions after the content, 24px apart and 8px from each other', () => {
    const card = mountDialog({ default: () => h('p', 'Delete it?'), actions: () => [h('button', 'Cancel'), h('button', 'Delete')] })
    const actions = card.querySelector<HTMLElement>('[data-dialog-actions]')!
    expect(actions.previousElementSibling!.textContent).toBe('Delete it?')
    expect(actions.classList).toContain('mt-6')
    expect(actions.classList).toContain('gap-2')
    expect([...actions.querySelectorAll('button')].map((button) => button.textContent)).toEqual(['Cancel', 'Delete'])
  })

  it('has no actions row without actions', () => {
    expect(mountDialog({ default: () => h('p', 'Done') }).querySelector('[data-dialog-actions]')).toBeNull()
  })
})

// Every dialog's own source, to check its buttons use the actions slot.
const sources = import.meta.glob<string>('/src/**/*.vue', { query: '?raw', import: 'default', eager: true })

describe('dialogs', () => {
  it('put their buttons in the actions slot, not a row of their own', () => {
    const offenders: string[] = []
    for (const [file, source] of Object.entries(sources)) {
      for (const dialog of source.match(/<AppDialog[\s\S]*?<\/AppDialog>/g) ?? []) {
        const outsideActions = dialog.replace(/<template #actions>[\s\S]*?<\/template>/g, '')
        if (/<AppButton/.test(outsideActions)) offenders.push(file)
      }
    }
    expect(offenders).toEqual([])
  })

  it('are all checked', () => {
    const dialogs = Object.values(sources).flatMap((source) => source.match(/<AppDialog[\s\S]*?<\/AppDialog>/g) ?? [])
    expect(dialogs.length).toBeGreaterThanOrEqual(10)
  })
})
