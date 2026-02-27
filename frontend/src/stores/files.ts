import { defineStore } from 'pinia'
import { ref } from 'vue'
import { files as filesApi } from '@/utils/api'
import type { FileEntry } from '@/types'

export const useFilesStore = defineStore('files', () => {
  const currentPath = ref('')
  const entries = ref<FileEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function navigate(path: string) {
    currentPath.value = path // update immediately so watchers see the new path before entries load
    loading.value = true
    error.value = null
    try {
      entries.value = await filesApi.list(path)
    } catch (e: unknown) {
      error.value = e instanceof Error ? e.message : 'Failed to load directory'
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
