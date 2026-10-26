import { ref } from 'vue'
import { ConflictError, type ConflictMode } from '@/utils/api'

export type ConflictChoice = 'replace' | 'rename' | null

const visible = ref(false)
const title = ref('')
const conflicts = ref<string[]>([])
let _resolve: ((c: ConflictChoice) => void) | null = null

export function useConflictDialog() {
  function prompt(dialogTitle: string, names: string[]): Promise<ConflictChoice> {
    title.value = dialogTitle
    conflicts.value = names
    visible.value = true
    return new Promise((resolve) => { _resolve = resolve })
  }

  function choose(choice: ConflictChoice) {
    visible.value = false
    _resolve?.(choice)
    _resolve = null
  }

  // Runs an operation that refuses to overwrite (throws ConflictError) with
  // 'error' first; on a conflict, asks how to resolve it and runs it again
  // with the choice. Resolves with the operation's result, or null when the
  // user cancels. Other errors are passed on.
  // knownConflicts: names already known to be taken (from a listing): asks
  // before running at all, so a large upload isn't sent just to be refused.
  async function runWithConflictPrompt<T>(
    dialogTitle: string,
    run: (mode: ConflictMode) => Promise<T>,
    knownConflicts: string[] = [],
  ): Promise<T | null> {
    if (knownConflicts.length) {
      const choice = await prompt(dialogTitle, knownConflicts)
      return choice ? run(choice) : null
    }
    try {
      return await run('error')
    } catch (err) {
      if (!(err instanceof ConflictError)) throw err
      const choice = await prompt(dialogTitle, err.conflicts)
      return choice ? run(choice) : null
    }
  }

  return { visible, title, conflicts, prompt, choose, runWithConflictPrompt }
}
