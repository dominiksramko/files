import { defineStore } from 'pinia'
import { ref } from 'vue'
import { files as filesApi, NotFoundError } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import type { FileEntry } from '@/types'

export const useFilesStore = defineStore('files', () => {
  const currentPath = ref('')
  const entries = ref<FileEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const { show: showToast } = useToast()

  // Returns the path actually navigated to (may differ from requested path if not found).
  async function navigate(path: string): Promise<string> {
    currentPath.value = path // update immediately so watchers see the new path before entries load
    loading.value = true
    error.value = null
    try {
      entries.value = await filesApi.list(path)
      return path
    } catch (e: unknown) {
      if (e instanceof NotFoundError) {
        showToast(e.message, 'error')
        const fallback = e.fallbackPath
        currentPath.value = fallback
        try {
          entries.value = await filesApi.list(fallback)
        } catch {
          entries.value = []
        }
        return fallback
      }
      error.value = e instanceof Error ? e.message : 'Failed to load directory'
      return path
    } finally {
      loading.value = false
    }
  }

  function refresh() {
    return navigate(currentPath.value)
  }

  function reset() {
    currentPath.value = ''
    entries.value = []
    error.value = null
  }

  return { currentPath, entries, loading, error, navigate, refresh, reset }
})
