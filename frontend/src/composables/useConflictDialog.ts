import { ref } from 'vue'

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

  return { visible, title, conflicts, prompt, choose }
}
