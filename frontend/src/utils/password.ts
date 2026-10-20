// Same minimum as the server (MinPasswordLength in handlers/auth.go).
export const MIN_PASSWORD_LENGTH = 8

// What's wrong with a new password and its confirmation, or '' if nothing.
// Length counts characters, not UTF-16 units.
export function newPasswordProblem(password: string, confirmation: string): string {
  if (password !== confirmation) return 'New passwords do not match'
  if ([...password].length < MIN_PASSWORD_LENGTH) return `Password must be at least ${MIN_PASSWORD_LENGTH} characters`
  return ''
}
