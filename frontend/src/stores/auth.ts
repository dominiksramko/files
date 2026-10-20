import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { auth as authApi } from '@/utils/api'
import { useFilesStore } from '@/stores/files'
import type { User } from '@/types'

// The session lives in an HttpOnly cookie that scripts can't read, so whether
// we're logged in is known only by asking the server (restoreSession).
export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  let sessionChecked = false

  const isAuthenticated = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.is_admin ?? false)

  // Forgets everything about whoever was logged in: their file listing and its
  // live-update stream (per-view state goes when the views unmount), so the
  // next person on this browser never sees any of it.
  function clearUserState() {
    user.value = null
    useFilesStore().reset()
  }

  async function login(username: string, password: string) {
    const res = await authApi.login(username, password)
    clearUserState()
    user.value = res.user
    sessionChecked = true
  }

  // Picks up an existing session once per page load.
  async function restoreSession() {
    if (sessionChecked) return
    sessionChecked = true
    try {
      user.value = await authApi.me()
    } catch {
      user.value = null
    }
  }

  async function logout() {
    try {
      await authApi.logout()
    } finally {
      clearUserState()
    }
  }

  return { user, isAuthenticated, isAdmin, login, restoreSession, logout }
})
