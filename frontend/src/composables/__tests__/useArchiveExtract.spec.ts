import { afterEach, describe, expect, it, vi } from 'vitest'
import { useArchiveExtract } from '@/composables/useArchiveExtract'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { ConflictError, files as filesApi, PasswordRequiredError } from '@/utils/api'

afterEach(() => vi.restoreAllMocks())

// The server's extract: refuses without password 'hunter2'; with conflicts,
// refuses mode 'error' too.
function fakeExtract({ conflicts = false } = {}) {
  return vi.spyOn(filesApi, 'extract').mockImplementation(async (_path, _dest, password, mode) => {
    if (password !== 'hunter2') throw new PasswordRequiredError('archive is password protected')
    if (conflicts && mode === 'error') throw new ConflictError(['photos/a.jpg'])
    return { message: 'extracted' }
  })
}

describe('extractArchive', () => {
  it('asks about conflicts and extracts again with the choice', async () => {
    const extract = fakeExtract({ conflicts: true })
    const { visible, conflicts, choose } = useConflictDialog()
    const result = useArchiveExtract().extractArchive('photos.zip', { password: 'hunter2' })
    await vi.waitFor(() => expect(visible.value).toBe(true))
    expect(conflicts.value).toEqual(['photos/a.jpg'])
    choose('rename')
    expect(await result).toEqual({ message: 'extracted' })
    expect(extract.mock.calls).toEqual([
      ['photos.zip', undefined, 'hunter2', 'error'],
      ['photos.zip', undefined, 'hunter2', 'rename'],
    ])
  })

  it('extracts without asking when no password is needed', async () => {
    const extract = vi.spyOn(filesApi, 'extract').mockResolvedValue({ message: 'extracted' })
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    expect(await extractArchive('photos.zip')).toEqual({ message: 'extracted' })
    expect(passwordPrompt.visible.value).toBe(false)
    expect(extract).toHaveBeenCalledOnce()
  })

  it('asks for the password once the server says it needs one, until it works', async () => {
    const extract = fakeExtract()
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    const result = extractArchive('locked.zip')
    await vi.waitFor(() => expect(passwordPrompt.visible.value).toBe(true))
    expect(passwordPrompt.wrong.value).toBe(false)

    passwordPrompt.submit('nope')
    await vi.waitFor(() => expect(passwordPrompt.wrong.value).toBe(true))
    expect(passwordPrompt.visible.value).toBe(true)

    passwordPrompt.submit('hunter2')
    expect(await result).toEqual({ message: 'extracted' })
    expect(passwordPrompt.visible.value).toBe(false)
    expect(extract.mock.calls.map((call) => call[2])).toEqual([undefined, 'nope', 'hunter2'])
  })

  it('asks before trying when the listing showed encrypted files', async () => {
    const extract = fakeExtract()
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    const result = extractArchive('locked.zip', { encrypted: true })
    await vi.waitFor(() => expect(passwordPrompt.visible.value).toBe(true))
    expect(extract).not.toHaveBeenCalled()
    passwordPrompt.submit('hunter2')
    expect(await result).toEqual({ message: 'extracted' })
    expect(extract).toHaveBeenCalledOnce()
  })

  it('says a password given beforehand was wrong', async () => {
    fakeExtract()
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    const result = extractArchive('locked.zip', { password: 'nope', encrypted: true })
    await vi.waitFor(() => expect(passwordPrompt.visible.value).toBe(true))
    expect(passwordPrompt.wrong.value).toBe(true)
    passwordPrompt.cancel()
    expect(await result).toBeNull()
  })

  it('closes the password dialog before asking about conflicts, then uses the password that worked', async () => {
    const extract = fakeExtract({ conflicts: true })
    const conflictDialog = useConflictDialog()
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    const result = extractArchive('locked.zip', { encrypted: true })
    await vi.waitFor(() => expect(passwordPrompt.visible.value).toBe(true))
    passwordPrompt.submit('hunter2')
    await vi.waitFor(() => expect(conflictDialog.visible.value).toBe(true))
    expect(passwordPrompt.visible.value).toBe(false)
    conflictDialog.choose('replace')
    expect(await result).toEqual({ message: 'extracted' })
    expect(extract.mock.calls.slice(-1)[0]).toEqual(['locked.zip', undefined, 'hunter2', 'replace'])
  })

  it('resolves null when the password is not given', async () => {
    fakeExtract()
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    const result = extractArchive('locked.zip')
    await vi.waitFor(() => expect(passwordPrompt.visible.value).toBe(true))
    passwordPrompt.cancel()
    expect(await result).toBeNull()
    expect(passwordPrompt.visible.value).toBe(false)
  })

  it('throws other errors without asking', async () => {
    vi.spyOn(filesApi, 'extract').mockRejectedValue(new Error('disk full'))
    const { extractArchive, passwordPrompt } = useArchiveExtract()
    await expect(extractArchive('photos.zip')).rejects.toThrow('disk full')
    expect(passwordPrompt.visible.value).toBe(false)
    expect(useConflictDialog().visible.value).toBe(false)
  })
})
