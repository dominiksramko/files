// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import router, { showLoginAfterLogout } from '@/router'
import { useFilesStore } from '@/stores/files'

beforeEach(() => setActivePinia(createPinia()))
afterEach(() => vi.restoreAllMocks())

describe('showLoginAfterLogout', () => {
  it('opens the login page first, then clears the files', async () => {
    const order: string[] = []
    vi.spyOn(router, 'push').mockImplementation(async (to) => { order.push(`push ${JSON.stringify(to)}`) })
    vi.spyOn(useFilesStore(), 'reset').mockImplementation(() => { order.push('reset') })

    await showLoginAfterLogout('/?path=docs&preview=docs/a.txt')
    expect(order).toEqual([
      `push ${JSON.stringify({ name: 'login', query: { redirect: '/?path=docs&preview=docs/a.txt' } })}`,
      'reset',
    ])
  })

  it('without a place to return to, opens the plain login page', async () => {
    const push = vi.spyOn(router, 'push').mockResolvedValue(undefined)
    await showLoginAfterLogout()
    expect(push).toHaveBeenCalledWith({ name: 'login', query: {} })
  })
})
