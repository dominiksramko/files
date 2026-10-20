// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { isTypingTarget } from '@/utils/keyboard'

describe('isTypingTarget', () => {
  it('is true for fields that take text', () => {
    for (const tag of ['input', 'textarea', 'select']) {
      expect(isTypingTarget(document.createElement(tag)), tag).toBe(true)
    }
  })

  it('is true for rich-text fields', () => {
    const editor = document.createElement('div')
    // jsdom doesn't compute isContentEditable; browsers do for contenteditable="true" and inside it.
    Object.defineProperty(editor, 'isContentEditable', { value: true })
    expect(isTypingTarget(editor)).toBe(true)
  })

  it('is false for the page and other elements', () => {
    expect(isTypingTarget(document.body)).toBe(false)
    expect(isTypingTarget(document.createElement('button'))).toBe(false)
    expect(isTypingTarget(null)).toBe(false)
    expect(isTypingTarget(window)).toBe(false)
  })
})
