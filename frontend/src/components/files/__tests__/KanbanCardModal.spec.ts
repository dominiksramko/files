// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import KanbanCardModal from '@/components/files/KanbanCardModal.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

// Mounts the editor on a card due on due, done or not; returns its models.
async function mountEditor(due: string | null, dueDone: boolean) {
  const models = { due: ref(due), dueDone: ref(dueDone) }
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => h(KanbanCardModal, {
      title: 'Ship', description: '', labels: [], checklist: [],
      due: models.due.value, 'onUpdate:due': (value: string | null) => { models.due.value = value },
      dueDone: models.dueDone.value, 'onUpdate:dueDone': (value: boolean) => { models.dueDone.value = value },
      columnId: 1, columnTitle: 'To do', columnOptions: [{ label: 'To do', value: 1 }], boardLabels: [], entryPath: 'a.kanban',
    }),
  })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await nextTick()
  return models
}

const doneLabel = () => [...document.querySelectorAll('label')].find((label) => label.textContent?.trim() === 'Done')

describe('KanbanCardModal due date', () => {
  it('marks the due date done, and not done again', async () => {
    const models = await mountEditor('2026-11-01', false)
    doneLabel()!.querySelector('button')!.click()
    expect(models.dueDone.value).toBe(true)
    await nextTick()
    doneLabel()!.querySelector('button')!.click()
    expect(models.dueDone.value).toBe(false)
  })

  it('offers Done only once there is a date', async () => {
    await mountEditor(null, false)
    expect(doneLabel()).toBeUndefined()
  })

  it('clears done along with the date, leaving no stray @done', async () => {
    const models = await mountEditor('2026-11-01', true)
    document.querySelector<HTMLButtonElement>('button[title="Clear due date"]')!.click()
    expect(models.due.value).toBeNull()
    expect(models.dueDone.value).toBe(false)
  })

  it('clears done when the date is emptied in the date field too', async () => {
    const models = await mountEditor('2026-11-01', true)
    const field = document.querySelector<HTMLInputElement>('input[type="date"]')!
    field.value = ''
    field.dispatchEvent(new Event('change'))
    expect(models.due.value).toBeNull()
    expect(models.dueDone.value).toBe(false)
  })
})
