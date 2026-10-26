// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import KanbanCard from '@/components/files/KanbanCard.vue'
import { parseKanban, type KanbanCard as KanbanCardData } from '@/utils/kanban'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

const cardOf = (line: string) => parseKanban(`## To do\n${line}\n`).columns[0]!.cards[0]!

// Mounts the card; returns the events it sent, in order.
async function mountCard(card: KanbanCardData) {
  const events: unknown[][] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => h(KanbanCard, {
      card,
      onOpen: () => events.push(['open']),
      onToggleDueDone: () => events.push(['toggleDueDone']),
      onMenu: (x: number, y: number) => events.push(['menu', x, y]),
    }),
  })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await nextTick()
  return { events, root }
}

describe('KanbanCard', () => {
  it('marks the due date done from its badge, without opening the card', async () => {
    const { events, root } = await mountCard(cardOf('- Ship @2026-11-01'))
    root.querySelector<HTMLButtonElement>('button[title="Click to mark done"]')!.click()
    expect(events).toEqual([['toggleDueDone']])
  })

  it('shows a done due date in green with a check, offering to mark it not done', async () => {
    const { root } = await mountCard(cardOf('- Ship @2000-01-01 @done'))
    const badge = root.querySelector<HTMLButtonElement>('button[title="Done — click to mark not done"]')!
    expect(badge.className).toContain('text-emerald-300')
    expect(badge.className).not.toContain('text-red-300')
  })

  it('opens the menu below its ⋯ button, without opening the card', async () => {
    const { events, root } = await mountCard(cardOf('- Ship'))
    const button = root.querySelector<HTMLButtonElement>('button[aria-label="Card actions"]')!
    button.getBoundingClientRect = () => ({ left: 40, bottom: 100 }) as DOMRect
    button.click()
    expect(events).toEqual([['menu', 40, 104]])
  })

  it('opens the menu where it is right-clicked, instead of the browser menu', async () => {
    const { events, root } = await mountCard(cardOf('- Ship'))
    const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 12, clientY: 34 })
    root.querySelector('[role="button"]')!.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    expect(events).toEqual([['menu', 12, 34]])
  })

  it('still opens when clicked anywhere else', async () => {
    const { events, root } = await mountCard(cardOf('- Ship @2026-11-01'))
    root.querySelector<HTMLElement>('[role="button"] p')!.click()
    expect(events).toEqual([['open']])
  })
})
