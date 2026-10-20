import { afterEach, describe, expect, it, vi } from 'vitest'
import { useArchiveExtract } from '@/composables/useArchiveExtract'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { ConflictError, files as filesApi } from '@/utils/api'

afterEach(() => vi.restoreAllMocks())

describe('extractArchive', () => {
  it('asks about conflicts and extracts again with the choice', async () => {
    const extract = vi.spyOn(filesApi, 'extract').mockImplementation(async (_path, _dest, _password, mode) => {
      if (mode === 'error') throw new ConflictError(['photos/a.jpg'])
      return { message: 'extracted' }
    })
    const { visible, conflicts, choose } = useConflictDialog()
    const result = useArchiveExtract().extractArchive('photos.zip', 'secret')
    await vi.waitFor(() => expect(visible.value).toBe(true))
    expect(conflicts.value).toEqual(['photos/a.jpg'])
    choose('rename')
    expect(await result).toEqual({ message: 'extracted' })
    expect(extract.mock.calls).toEqual([
      ['photos.zip', undefined, 'secret', 'error'],
      ['photos.zip', undefined, 'secret', 'rename'],
    ])
  })

  it('throws a wrong password without asking', async () => {
    vi.spyOn(filesApi, 'extract').mockRejectedValue(new Error('wrong password'))
    await expect(useArchiveExtract().extractArchive('locked.7z', 'nope')).rejects.toThrow('wrong password')
    expect(useConflictDialog().visible.value).toBe(false)
  })
})
