import { describe, expect, it, vi } from 'vitest'
import { usePasswordPrompt } from '@/composables/usePasswordPrompt'
import { PasswordRequiredError } from '@/utils/api'

describe('usePasswordPrompt', () => {
  it('keeps the dialog open while a password is tried, and cancelling waits for it', async () => {
    let finish: (value: string) => void = () => {}
    const prompt = usePasswordPrompt()
    const result = prompt.runWithPasswordPrompt((password) =>
      password ? new Promise<string>((resolve) => { finish = resolve }) : Promise.reject(new PasswordRequiredError()),
    )
    await vi.waitFor(() => expect(prompt.visible.value).toBe(true))
    prompt.submit('hunter2')
    expect(prompt.busy.value).toBe(true)
    prompt.cancel()
    prompt.submit('again')
    expect(prompt.visible.value).toBe(true)
    finish('done')
    expect(await result).toBe('done')
    expect(prompt.visible.value).toBe(false)
    expect(prompt.busy.value).toBe(false)
  })

  it('ignores an empty password', async () => {
    const run = vi.fn((password?: string) => (password ? Promise.resolve('done') : Promise.reject(new PasswordRequiredError())))
    const prompt = usePasswordPrompt()
    const result = prompt.runWithPasswordPrompt(run)
    await vi.waitFor(() => expect(prompt.visible.value).toBe(true))
    prompt.submit('')
    expect(run).toHaveBeenCalledOnce()
    prompt.submit('x')
    expect(await result).toBe('done')
  })

  it('closes and passes on an error that is not about the password', async () => {
    const prompt = usePasswordPrompt()
    const result = prompt.runWithPasswordPrompt((password) =>
      Promise.reject(password ? new Error('disk full') : new PasswordRequiredError()),
    )
    await vi.waitFor(() => expect(prompt.visible.value).toBe(true))
    prompt.submit('hunter2')
    await expect(result).rejects.toThrow('disk full')
    expect(prompt.visible.value).toBe(false)
  })

  it('gives each user its own dialog', async () => {
    const first = usePasswordPrompt()
    const second = usePasswordPrompt()
    void first.runWithPasswordPrompt(() => Promise.reject(new PasswordRequiredError()))
    await vi.waitFor(() => expect(first.visible.value).toBe(true))
    expect(second.visible.value).toBe(false)
  })
})
