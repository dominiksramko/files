// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { FileEntry } from '@/types'
import KanbanCardModal from '@/components/files/KanbanCardModal.vue'
import { files as filesApi } from '@/utils/api'

const cleanups: (() => void)[] = []
beforeEach(() => {
  // The file chooser's breadcrumb measures itself; jsdom has no layout.
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
})
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

interface EditorOptions {
  due?: string | null
  dueDone?: boolean
  attachments?: string[]
  files?: Record<string, FileEntry | null> // attachmentEntry's answers by path
  uploadPercent?: number | null
}

// Mounts the editor on a card of the board work/Roadmap.kanban; returns its
// models and the events it sent.
async function mountEditor({ due = null, dueDone = false, attachments = [], files = {}, uploadPercent = null }: EditorOptions = {}) {
  const models = { due: ref(due), dueDone: ref(dueDone), attachments: ref(attachments) }
  const events: unknown[][] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => h(KanbanCardModal, {
      title: 'Ship', description: '', labels: [], checklist: [],
      due: models.due.value, 'onUpdate:due': (value: string | null) => { models.due.value = value },
      dueDone: models.dueDone.value, 'onUpdate:dueDone': (value: boolean) => { models.dueDone.value = value },
      attachments: models.attachments.value, 'onUpdate:attachments': (value: string[]) => { models.attachments.value = value },
      columnId: 1, columnTitle: 'To do', columnOptions: [{ label: 'To do', value: 1 }], boardLabels: [],
      entryPath: 'work/Roadmap.kanban',
      attachmentEntry: (path: string) => files[path],
      uploadPercent,
      onUpload: (uploaded: File[]) => events.push(['upload', uploaded.map((file) => file.name)]),
      onAttach: (paths: string[]) => events.push(['attach', paths]),
      onOpenAttachment: (path: string) => events.push(['openAttachment', path]),
    }),
  })
  app.use(createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }] }))
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await nextTick()
  return { models, events }
}

const doneLabel = () => [...document.querySelectorAll('label')].find((label) => label.textContent?.trim() === 'Done')

describe('KanbanCardModal due date', () => {
  it('marks the due date done, and not done again', async () => {
    const { models } = await mountEditor({ due: '2026-11-01' })
    doneLabel()!.querySelector('button')!.click()
    expect(models.dueDone.value).toBe(true)
    await nextTick()
    doneLabel()!.querySelector('button')!.click()
    expect(models.dueDone.value).toBe(false)
  })

  it('offers Done only once there is a date', async () => {
    await mountEditor()
    expect(doneLabel()).toBeUndefined()
  })

  it('clears done along with the date, leaving no stray @done', async () => {
    const { models } = await mountEditor({ due: '2026-11-01', dueDone: true })
    document.querySelector<HTMLButtonElement>('button[title="Clear due date"]')!.click()
    expect(models.due.value).toBeNull()
    expect(models.dueDone.value).toBe(false)
  })

  it('clears done when the date is emptied in the date field too', async () => {
    const { models } = await mountEditor({ due: '2026-11-01', dueDone: true })
    const field = document.querySelector<HTMLInputElement>('input[type="date"]')!
    field.value = ''
    field.dispatchEvent(new Event('change'))
    expect(models.due.value).toBeNull()
    expect(models.dueDone.value).toBe(false)
  })
})

function fileEntry(path: string, mime: string, size = 2048): FileEntry {
  return { name: path.split('/').pop()!, path, is_dir: false, size, mod_time: '2026-10-26T10:00:00Z', mime, has_thumb: false }
}

const attachmentRows = () => [...document.querySelectorAll<HTMLElement>('[data-attachments] li')]

describe('KanbanCardModal attachments', () => {
  const files = {
    'work/Roadmap files/mockup.png': fileEntry('work/Roadmap files/mockup.png', 'image/png'),
    'specs/login.pdf': fileEntry('specs/login.pdf', 'application/pdf', 1024 * 1024),
    'work/gone.txt': null,
  }

  it('lists each attached file with a picture of an image, its size and where it is', async () => {
    await mountEditor({ attachments: ['Roadmap files/mockup.png', '../specs/login.pdf'], files })
    const [mockup, spec] = attachmentRows()
    expect(mockup!.textContent).toContain('mockup.png')
    expect(mockup!.textContent).toContain('2.0 KB · Roadmap files')
    expect(mockup!.querySelector('img')!.getAttribute('src')).toContain('path=work%2FRoadmap%20files%2Fmockup.png')
    expect(spec!.textContent).toContain('1.0 MB · ../specs')
    expect(spec!.querySelector('img')).toBeNull()
  })

  it('says where a missing file was looked for, and opens nothing for it', async () => {
    const { events } = await mountEditor({ attachments: ['gone.txt'], files })
    const [row] = attachmentRows()
    expect(row!.textContent).toContain('Not found at gone.txt')
    row!.click()
    expect(events).toEqual([])
  })

  it('opens a file by its path in the user\'s folder', async () => {
    const { events } = await mountEditor({ attachments: ['../specs/login.pdf'], files })
    attachmentRows()[0]!.click()
    expect(events).toEqual([['openAttachment', 'specs/login.pdf']])
  })

  it('removes only the attachment clicked, without opening it, even when a file is listed twice', async () => {
    const { models, events } = await mountEditor({ attachments: ['gone.txt', '../specs/login.pdf', 'gone.txt'], files })
    attachmentRows()[2]!.querySelector<HTMLButtonElement>('button[aria-label="Remove gone.txt"]')!.click()
    expect(models.attachments.value).toEqual(['gone.txt', '../specs/login.pdf'])
    expect(events).toEqual([])
  })

  it('uploads the files picked', async () => {
    const { events } = await mountEditor()
    const input = document.querySelector<HTMLInputElement>('[data-attachments] input[type="file"]')!
    Object.defineProperty(input, 'files', { value: [new File(['a'], 'a.png'), new File(['b'], 'b.pdf')], configurable: true })
    input.dispatchEvent(new Event('change'))
    expect(events).toEqual([['upload', ['a.png', 'b.pdf']]])
  })

  it('uploads files dropped on the card, leaving out folders', async () => {
    const { events } = await mountEditor()
    const item = (name: string, isDirectory = false) => ({
      kind: 'file', getAsFile: () => new File(['x'], name), webkitGetAsEntry: () => ({ name, isDirectory }),
    })
    const drop = new Event('drop', { bubbles: true, cancelable: true })
    Object.defineProperty(drop, 'dataTransfer', { value: { types: ['Files'], items: [item('a.png'), item('photos', true)] } })
    document.querySelector('[data-attachments]')!.dispatchEvent(drop)
    expect(drop.defaultPrevented).toBe(true)
    expect(events).toEqual([['upload', ['a.png']]])
  })

  it('shows how far an upload has got', async () => {
    await mountEditor({ uploadPercent: 42 })
    expect(document.querySelector('[data-attachments]')!.textContent).toContain('Uploading… 42%')
  })

  it("attaches files chosen from the user's folder, starting in the board's folder", async () => {
    const list = vi.spyOn(filesApi, 'list').mockResolvedValue([fileEntry('work/notes.txt', 'text/plain'), fileEntry('work/plan.pdf', 'application/pdf')])
    const { events } = await mountEditor()
    const chooseButton = [...document.querySelectorAll('button')].find((button) => button.textContent?.includes('Choose from my files'))!
    chooseButton.click()
    await vi.waitFor(() => expect(document.querySelector('[data-picker-entries]')).not.toBeNull())
    expect(list).toHaveBeenCalledWith('work')

    const row = [...document.querySelectorAll<HTMLElement>('[data-picker-entries] [role="button"]')].find((element) => element.textContent?.includes('plan.pdf'))!
    row.click()
    await nextTick()
    const attachButton = [...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === 'Attach')!
    attachButton.click()
    expect(events).toEqual([['attach', ['work/plan.pdf']]])
    await nextTick()
    expect(document.querySelector('[data-picker-entries]')).toBeNull()
  })
})
