import { describe, expect, it } from 'vitest'
import { queryString, withKeys } from '@/utils/query'

describe('queryString', () => {
  it('reads a key, telling a bare key from a missing one', () => {
    expect(queryString('docs')).toBe('docs')
    expect(queryString(null)).toBe('')
    expect(queryString(undefined)).toBeUndefined()
  })

  it('takes the first of a repeated key', () => {
    expect(queryString(['one', 'two'])).toBe('one')
    expect(queryString([null])).toBe('')
  })
})

describe('withKeys', () => {
  const keys = ['modal', 'modal_at']

  it('sets the keys and keeps the others', () => {
    expect(withKeys({ path: 'docs', modal_at: 'old' }, keys, { modal: 'a', modal_at: 'b' }))
      .toEqual({ path: 'docs', modal: 'a', modal_at: 'b' })
  })

  it('drops keys left undefined, and only its own keys', () => {
    expect(withKeys({ path: 'docs', modal: 'a', modal_at: 'b' }, keys, { modal: 'a', modal_at: undefined }))
      .toEqual({ path: 'docs', modal: 'a' })
    expect(withKeys({ path: 'docs', modal: 'a' }, keys, {})).toEqual({ path: 'docs' })
  })

  it('keeps an empty value', () => {
    expect(withKeys({}, keys, { modal: '' })).toEqual({ modal: '' })
  })
})
