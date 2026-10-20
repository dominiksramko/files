import { describe, expect, it } from 'vitest'
import { newPasswordProblem } from '@/utils/password'

describe('newPasswordProblem', () => {
  it('accepts 8 characters or more that match', () => {
    expect(newPasswordProblem('12345678', '12345678')).toBe('')
    expect(newPasswordProblem('ünïcödé!', 'ünïcödé!')).toBe('')
  })

  it('refuses fewer than 8 characters', () => {
    expect(newPasswordProblem('1234567', '1234567')).toBe('Password must be at least 8 characters')
    expect(newPasswordProblem('🔑🔑🔑🔑🔑🔑🔑', '🔑🔑🔑🔑🔑🔑🔑')).not.toBe('') // 7 characters, 14 UTF-16 units
  })

  it('refuses a confirmation that differs', () => {
    expect(newPasswordProblem('long enough', 'long enougg')).toBe('New passwords do not match')
  })
})
