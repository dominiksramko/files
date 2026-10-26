// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent } from 'vue'
import type { FileEntry } from '@/types'
import { useAttachmentUploads } from '@/composables/useAttachmentUploads'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { useToast } from '@/composables/useToast'
import { ConflictError, fileops, type ConflictMode } from '@/utils/api'

const cleanups: (() => void)[] = []
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.restoreAllMocks()
})

const FOLDER = 'work/Roadmap files'

// The server's upload into a folder already holding taken; 'rename' adds " (1)".
function fakeUpload(taken: string[] = []) {
  const aborts: ReturnType<typeof vi.fn>[] = []
  const finishers: (() => void)[] = []
  const upload = vi.spyOn(fileops, 'upload').mockImplementation((_folder, files, onProgress, mode: ConflictMode = 'error') => {
    const abort = vi.fn()
    aborts.push(abort)
    const promise = new Promise<{ message: string; count: number; names: string[] }>((resolve, reject) => {
      const conflicts = files.map((file) => file.name).filter((name) => taken.includes(name))
      if (mode === 'error' && conflicts.length) return reject(new ConflictError(conflicts))
      onProgress?.(50, 100)
      finishers.push(() => resolve({
        message: 'uploaded',
        count: files.length,
        names: files.map((file) => (mode === 'rename' && taken.includes(file.name) ? file.name.replace('.', ' (1).') : file.name)),
      }))
    })
    return { promise, abort }
  })
  const finishAll = () => finishers.splice(0).forEach((finish) => finish())
  return { upload, aborts, finishAll }
}

function mountUploads(known: Record<string, FileEntry> = {}) {
  const attached: [number, string[]][] = []
  let uploads!: ReturnType<typeof useAttachmentUploads>
  const app = createApp(defineComponent({
    setup() {
      uploads = useAttachmentUploads({
        folder: () => FOLDER,
        entryFor: (path) => known[path] ?? null,
        attach: (cardId, paths) => attached.push([cardId, paths]),
      })
      return () => null
    },
  }))
  app.mount(document.createElement('div'))
  cleanups.push(() => app.unmount())
  return { uploads, attached, unmount: () => app.unmount() }
}

const file = (name: string) => new File(['content'], name)

describe('useAttachmentUploads', () => {
  it('uploads into the folder and attaches the files to the card, showing progress meanwhile', async () => {
    const { upload, finishAll } = fakeUpload()
    const { uploads, attached } = mountUploads()
    const done = uploads.upload(7, [file('a.png'), file('b.pdf')])
    await vi.waitFor(() => expect(uploads.percents.get(7)).toBe(50))
    finishAll()
    await done

    expect(upload.mock.calls[0]![0]).toBe(FOLDER)
    expect(attached).toEqual([[7, [`${FOLDER}/a.png`, `${FOLDER}/b.pdf`]]])
    expect(uploads.percents.has(7)).toBe(false)
  })

  it('asks about a name the server finds taken, and attaches the file under the name it was kept as', async () => {
    const { upload, finishAll } = fakeUpload(['a.png'])
    const { uploads, attached } = mountUploads()
    const { visible, conflicts, choose } = useConflictDialog()
    const done = uploads.upload(7, [file('a.png')])
    await vi.waitFor(() => expect(visible.value).toBe(true))
    expect(conflicts.value).toEqual(['a.png'])
    choose('rename')
    await vi.waitFor(() => expect(upload).toHaveBeenCalledTimes(2))
    finishAll()
    await done
    expect(attached).toEqual([[7, [`${FOLDER}/a (1).png`]]])
  })

  it('asks before sending anything about a name known to be taken', async () => {
    const { upload, finishAll } = fakeUpload(['a.png'])
    const known = { [`${FOLDER}/a.png`]: { name: 'a.png', path: `${FOLDER}/a.png`, is_dir: false, size: 1, mod_time: '', mime: 'image/png', has_thumb: false } }
    const { uploads } = mountUploads(known)
    const { visible, choose } = useConflictDialog()
    const done = uploads.upload(7, [file('a.png')])
    await vi.waitFor(() => expect(visible.value).toBe(true))
    expect(upload).not.toHaveBeenCalled()
    choose('replace')
    await vi.waitFor(() => expect(upload).toHaveBeenCalledOnce())
    expect(upload.mock.calls[0]![3]).toBe('replace')
    finishAll()
    await done
  })

  it('attaches nothing when the user cancels', async () => {
    fakeUpload(['a.png'])
    const { uploads, attached } = mountUploads()
    const { visible, choose } = useConflictDialog()
    const done = uploads.upload(7, [file('a.png')])
    await vi.waitFor(() => expect(visible.value).toBe(true))
    choose(null)
    await done
    expect(attached).toEqual([])
    expect(uploads.percents.has(7)).toBe(false)
  })

  it('says what went wrong, attaching nothing', async () => {
    vi.spyOn(fileops, 'upload').mockReturnValue({ promise: Promise.reject(new Error('disk full')), abort: vi.fn() })
    const { uploads, attached } = mountUploads()
    await uploads.upload(7, [file('a.png')])
    expect(useToast().toast.value?.message).toBe('disk full')
    expect(attached).toEqual([])
  })

  it('stops uploads still running when the board closes', async () => {
    const { aborts } = fakeUpload()
    const { uploads, unmount } = mountUploads()
    void uploads.upload(7, [file('a.png')])
    void uploads.upload(8, [file('b.png')])
    await vi.waitFor(() => expect(aborts).toHaveLength(2))
    cleanups.splice(0)
    unmount()
    expect(aborts.map((abort) => abort.mock.calls.length)).toEqual([1, 1])
  })
})
