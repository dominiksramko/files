import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { ConflictError, type ConflictMode } from '@/utils/api'

// An operation that refuses with a conflict on 'error' and succeeds otherwise.
function conflictingOperation() {
  return vi.fn(async (mode: ConflictMode) => {
    if (mode === 'error') throw new ConflictError(['notes.txt'])
    return { name: mode === 'rename' ? 'notes (1).txt' : 'notes.txt' }
  })
}

describe('runWithConflictPrompt', () => {
  it('runs once when nothing conflicts', async () => {
    const { runWithConflictPrompt, visible } = useConflictDialog()
    const run = vi.fn(async () => 'done')
    expect(await runWithConflictPrompt('Title', run)).toBe('done')
    expect(run).toHaveBeenCalledExactlyOnceWith('error')
    expect(visible.value).toBe(false)
  })

  for (const choice of ['replace', 'rename'] as const) {
    it(`asks on a conflict and runs again with '${choice}'`, async () => {
      const { runWithConflictPrompt, visible, title, conflicts, choose } = useConflictDialog()
      const run = conflictingOperation()
      const result = runWithConflictPrompt('Rename conflict', run)
      await vi.waitFor(() => expect(visible.value).toBe(true))
      expect(title.value).toBe('Rename conflict')
      expect(conflicts.value).toEqual(['notes.txt'])
      choose(choice)
      expect(await result).toEqual({ name: choice === 'rename' ? 'notes (1).txt' : 'notes.txt' })
      expect(run.mock.calls).toEqual([['error'], [choice]])
    })
  }

  it('resolves null when the user cancels', async () => {
    const { runWithConflictPrompt, visible, choose } = useConflictDialog()
    const run = conflictingOperation()
    const result = runWithConflictPrompt('Title', run)
    await vi.waitFor(() => expect(visible.value).toBe(true))
    choose(null)
    expect(await result).toBeNull()
    expect(run).toHaveBeenCalledOnce()
  })

  it('asks first about names known to be taken, then runs once with the choice', async () => {
    const { runWithConflictPrompt, visible, conflicts, choose } = useConflictDialog()
    const run = vi.fn(async (mode: ConflictMode) => mode)
    const result = runWithConflictPrompt('Upload conflicts', run, ['a.png'])
    await vi.waitFor(() => expect(visible.value).toBe(true))
    expect(conflicts.value).toEqual(['a.png'])
    expect(run).not.toHaveBeenCalled()
    choose('rename')
    expect(await result).toBe('rename')
    expect(run).toHaveBeenCalledExactlyOnceWith('rename')
  })

  it("doesn't run at all when the user cancels over known names", async () => {
    const { runWithConflictPrompt, visible, choose } = useConflictDialog()
    const run = vi.fn(async () => 'done')
    const result = runWithConflictPrompt('Upload conflicts', run, ['a.png'])
    await vi.waitFor(() => expect(visible.value).toBe(true))
    choose(null)
    expect(await result).toBeNull()
    expect(run).not.toHaveBeenCalled()
  })

  it('passes other errors on without asking', async () => {
    const { runWithConflictPrompt, visible } = useConflictDialog()
    await expect(runWithConflictPrompt('Title', async () => { throw new Error('wrong password') })).rejects.toThrow('wrong password')
    await nextTick()
    expect(visible.value).toBe(false)
  })
})
