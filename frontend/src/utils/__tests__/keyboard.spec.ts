// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { isModalOpen, isTypingTarget } from '@/utils/keyboard'

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

describe('isModalOpen', () => {
  it('is true only while a modal dialog is in the page', () => {
    expect(isModalOpen()).toBe(false)
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    document.body.append(dialog)
    expect(isModalOpen()).toBe(true)
    dialog.remove()
    expect(isModalOpen()).toBe(false)
  })
})
