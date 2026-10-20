import { describe, expect, it } from 'vitest'
import { nextTick, ref } from 'vue'
import { useJobSelection } from '@/composables/useJobSelection'
import type { FileEntry } from '@/types'

function entry(path: string): FileEntry {
  return { name: path, path, is_dir: false, size: 0, mod_time: '', mime: 'text/plain' } as FileEntry
}

function setup(paths: string[]) {
  const selectedPaths = ref<string[]>([...paths])
  const entries = ref<FileEntry[]>(paths.map(entry))
  return { selectedPaths, entries, ...useJobSelection(selectedPaths, () => entries.value) }
}

describe('useJobSelection', () => {
  it('clears the selection when the job finishes', () => {
    const { selectedPaths, trackJob, clearSelectionAfterJob } = setup(['a', 'b'])
    trackJob(1)
    clearSelectionAfterJob(1)
    expect(selectedPaths.value).toEqual([])
  })

  it('keeps a selection the user changed meanwhile', () => {
    const { selectedPaths, trackJob, clearSelectionAfterJob } = setup(['a', 'b'])
    trackJob(1)
    selectedPaths.value = ['a']
    clearSelectionAfterJob(1)
    expect(selectedPaths.value).toEqual(['a'])
  })

  it('only lets the last job started clear it', () => {
    const { selectedPaths, trackJob, clearSelectionAfterJob } = setup(['a'])
    trackJob(1)
    trackJob(2)
    clearSelectionAfterJob(1)
    expect(selectedPaths.value).toEqual(['a'])
    clearSelectionAfterJob(2)
    expect(selectedPaths.value).toEqual([])
  })

  it('drops removed files from the selection without counting it as a change', async () => {
    const { selectedPaths, entries, trackJob, clearSelectionAfterJob } = setup(['a', 'b', 'c'])
    trackJob(1)
    entries.value = [entry('a'), entry('c')]
    await nextTick()
    expect(selectedPaths.value).toEqual(['a', 'c'])
    clearSelectionAfterJob(1)
    expect(selectedPaths.value).toEqual([])
  })
})
