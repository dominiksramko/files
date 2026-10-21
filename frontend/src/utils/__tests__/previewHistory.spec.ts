// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { createRouter, createWebHistory } from 'vue-router'
import { PREVIEW_OPENED_STATE, previewHistoryStep } from '@/utils/previewHistory'

describe('previewHistoryStep', () => {
  it('adds a history entry when the preview opens', () => {
    expect(previewHistoryStep('a.jpg', null, false)).toBe('push')
  })

  it('replaces the entry when moving to another file', () => {
    expect(previewHistoryStep('b.jpg', 'a.jpg', true)).toBe('replace')
  })

  it('goes back to the folder when closing a preview it opened', () => {
    expect(previewHistoryStep(null, 'a.jpg', true)).toBe('back')
  })

  it('stays put when closing a preview opened from a link (there may be nothing to go back to)', () => {
    expect(previewHistoryStep(null, 'a.jpg', false)).toBe('replace')
  })

  it('does nothing when the URL already says so (Back or Forward pressed)', () => {
    expect(previewHistoryStep('a.jpg', 'a.jpg', true)).toBe('none')
    expect(previewHistoryStep(null, null, false)).toBe('none')
  })
})

describe('the router keeps the preview mark', () => {
  it('through moving between files, not into new entries', async () => {
    const router = createRouter({ history: createWebHistory(), routes: [{ path: '/', component: { render: () => null } }] })
    await router.push({ query: { path: 'photos' } })
    await router.push({ query: { path: 'photos', preview: 'photos/a.jpg' }, state: { [PREVIEW_OPENED_STATE]: true } })
    await router.replace({ query: { path: 'photos', preview: 'photos/b.jpg' } })
    expect(router.options.history.state[PREVIEW_OPENED_STATE]).toBe(true)

    await router.push({ query: { path: 'other' } })
    expect(router.options.history.state[PREVIEW_OPENED_STATE]).toBeUndefined()
  })
})
