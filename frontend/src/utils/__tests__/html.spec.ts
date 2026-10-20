import { describe, expect, it } from 'vitest'
import { escapeHtml } from '@/utils/html'

describe('escapeHtml', () => {
  it('escapes every character with a meaning in HTML', () => {
    expect(escapeHtml(`<a href="x" title='y'>Tom & Jerry</a>`)).toBe(
      '&lt;a href=&quot;x&quot; title=&#39;y&#39;&gt;Tom &amp; Jerry&lt;/a&gt;',
    )
  })

  it('escapes an existing entity again, so it shows as typed', () => {
    expect(escapeHtml('&amp;')).toBe('&amp;amp;')
  })

  it('leaves other text alone', () => {
    expect(escapeHtml('plain text, ünïcode ✓')).toBe('plain text, ünïcode ✓')
  })
})
