import { ref } from 'vue'
import { debounce } from '@/utils/debounce'

export type ToastKind = 'success' | 'error' | 'info'

interface Toast {
  message: string
  kind: ToastKind
}

// Module-level singleton — shared across all components
const toast = ref<Toast | null>(null)
const clearToast = debounce(() => { toast.value = null }, 3500)

export function useToast() {
  function show(message: string, kind: ToastKind = 'info') {
    toast.value = { message, kind }
    clearToast()
  }

  return { toast, show }
}
