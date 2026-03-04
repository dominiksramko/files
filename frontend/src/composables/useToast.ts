import { ref } from 'vue'

export type ToastKind = 'success' | 'error' | 'info'

interface Toast {
  message: string
  kind: ToastKind
}

// Module-level singleton — shared across all components
const toast = ref<Toast | null>(null)
let timer: ReturnType<typeof setTimeout> | null = null

export function useToast() {
  function show(message: string, kind: ToastKind = 'info') {
    if (timer) clearTimeout(timer)
    toast.value = { message, kind }
    timer = setTimeout(() => { toast.value = null }, 3500)
  }

  return { toast, show }
}
