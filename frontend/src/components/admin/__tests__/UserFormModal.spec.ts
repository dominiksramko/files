// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import type { User } from '@/types'
import UserFormModal from '@/components/admin/UserFormModal.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

// Mounts the form for user (null: a new one); returns how often it closed.
async function mountForm(user: User | null) {
  const closes: number[] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({ render: () => h(UserFormModal, { user, onClose: () => closes.push(1) }) })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  await nextTick()
  await nextTick()
  return closes
}

const closeButton = () => document.querySelector<HTMLButtonElement>('[data-modal-close]')!
const input = (placeholder: string) => document.querySelector<HTMLInputElement>(`input[placeholder="${placeholder}"]`)!

describe('UserFormModal', () => {
  it('closes at once when nothing was typed', async () => {
    const closes = await mountForm(null)
    closeButton().click()
    expect(closes).toHaveLength(1)
  })

  it('asks before losing what was typed', async () => {
    const closes = await mountForm(null)
    input('username').value = 'ann'
    input('username').dispatchEvent(new Event('input'))
    await nextTick()
    closeButton().click()
    await nextTick()
    expect(document.body.textContent).toContain('Discard changes?')
    expect(closes).toHaveLength(0)
  })

  it('does not count the user it opened with as a change', async () => {
    const user = { id: 2, username: 'ann', display_name: 'Ann', root_dir: '/data/ann', is_admin: false } as User
    const closes = await mountForm(user)
    closeButton().click()
    expect(closes).toHaveLength(1)
  })

  it('puts the cursor in the first field it can change', async () => {
    await mountForm(null)
    expect(document.activeElement).toBe(input('username'))
    cleanups.splice(0).forEach((cleanup) => cleanup())
    vi.restoreAllMocks()
    await mountForm({ id: 2, username: 'ann', display_name: '', root_dir: '', is_admin: false } as User)
    expect(document.activeElement).toBe(input('Alice Smith'))
  })
})
