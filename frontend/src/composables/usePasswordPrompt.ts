import { ref } from 'vue'
import { PasswordRequiredError } from '@/utils/api'

// Asks for a password when an operation needs one, in a PasswordDialog the
// caller renders (inside its own modal, so the dialog is on top of it). Each
// call of usePasswordPrompt has its own dialog.
export function usePasswordPrompt() {
  const visible = ref(false)
  const busy = ref(false) // an attempt with the typed password is running
  const wrong = ref(false) // the last password tried was refused
  let attempt: ((password: string) => Promise<void>) | null = null
  let cancelled: (() => void) | null = null

  function close() {
    visible.value = false
    attempt = null
    cancelled = null
  }

  // Runs an operation that throws PasswordRequiredError without the right
  // password: first with password (if given, or unless askFirst says one is
  // needed anyway), then with each password typed until one works. The
  // dialog stays open while an attempt runs and closes on any other outcome,
  // which is passed on. Resolves null when the user cancels.
  async function runWithPasswordPrompt<T>(
    run: (password?: string) => Promise<T>,
    { password, askFirst = false }: { password?: string; askFirst?: boolean } = {},
  ): Promise<T | null> {
    wrong.value = false
    if (password || !askFirst) {
      try {
        return await run(password)
      } catch (err) {
        if (!(err instanceof PasswordRequiredError)) throw err
        wrong.value = !!password
      }
    }
    return new Promise<T | null>((resolve, reject) => {
      attempt = async (typed) => {
        busy.value = true
        try {
          const result = await run(typed)
          close()
          resolve(result)
        } catch (err) {
          if (err instanceof PasswordRequiredError) {
            wrong.value = true
            return
          }
          close()
          reject(err)
        } finally {
          busy.value = false
        }
      }
      cancelled = () => {
        close()
        resolve(null)
      }
      visible.value = true
    })
  }

  function submit(password: string) {
    if (!password || busy.value) return
    attempt?.(password)
  }

  // Cancelling waits for a running attempt: its outcome decides.
  function cancel() {
    if (!busy.value) cancelled?.()
  }

  return { visible, busy, wrong, submit, cancel, runWithPasswordPrompt }
}

export type PasswordPrompt = ReturnType<typeof usePasswordPrompt>
