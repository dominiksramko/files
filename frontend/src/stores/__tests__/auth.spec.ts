// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import { useFilesStore } from '@/stores/files'
import { auth as authApi } from '@/utils/api'
import type { User } from '@/types'

const ann = { id: 2, username: 'ann', is_admin: false } as User

beforeEach(() => setActivePinia(createPinia()))
afterEach(() => vi.restoreAllMocks())

describe('auth store', () => {
  it('logs in', async () => {
    vi.spyOn(authApi, 'login').mockResolvedValue({ user: ann })
    const auth = useAuthStore()
    await auth.login('ann', 'pw')
    expect(auth.user).toEqual(ann)
    expect(auth.isAuthenticated).toBe(true)
  })

  it('asks the server about the session once per page load', async () => {
    const me = vi.spyOn(authApi, 'me').mockResolvedValue(ann)
    const auth = useAuthStore()
    await auth.restoreSession()
    await auth.restoreSession()
    expect(me).toHaveBeenCalledOnce()
    expect(auth.user).toEqual(ann)
  })

  it('is logged out when the server knows no session', async () => {
    vi.spyOn(authApi, 'me').mockRejectedValue(new Error('not logged in'))
    const auth = useAuthStore()
    await auth.restoreSession()
    expect(auth.isAuthenticated).toBe(false)
  })

  it('logs out on the server, and locally even if that fails', async () => {
    vi.spyOn(authApi, 'login').mockResolvedValue({ user: ann })
    const logout = vi.spyOn(authApi, 'logout').mockRejectedValue(new Error('offline'))
    const auth = useAuthStore()
    await auth.login('ann', 'pw')
    await expect(auth.logout()).rejects.toThrow('offline')
    expect(logout).toHaveBeenCalledOnce()
    expect(auth.user).toBeNull()
  })

  it("clearUserState drops the user's files; a new login starts clean", async () => {
    vi.spyOn(authApi, 'login').mockResolvedValue({ user: ann })
    const auth = useAuthStore()
    const reset = vi.spyOn(useFilesStore(), 'reset')
    await auth.login('ann', 'pw')
    expect(reset).toHaveBeenCalledOnce()
    auth.clearUserState()
    expect(auth.user).toBeNull()
    expect(reset).toHaveBeenCalledTimes(2)
  })

  it('logs out when the server says the session ended', async () => {
    vi.spyOn(authApi, 'login').mockResolvedValue({ user: ann })
    const auth = useAuthStore()
    expect(auth.sessionEnded()).toBe(false) // nobody logged in: e.g. a wrong password
    await auth.login('ann', 'pw')
    expect(auth.sessionEnded()).toBe(true)
    expect(auth.user).toBeNull()
  })
})
