// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { renderMarkdown } from '@/utils/markdown'

// Renders source as notes/readme.md and returns the result as DOM, the way v-html would insert it.
function render(source: string): HTMLElement {
  const container = document.createElement('div')
  container.innerHTML = renderMarkdown(source, 'notes/readme.md')
  return container
}

const allElements = (container: HTMLElement) => [container, ...container.querySelectorAll('*')]

describe('renderMarkdown removes anything that could run code or cover the app', () => {
  const attacks: Record<string, string> = {
    'event handler on raw HTML': '<img src="x" onerror="alert(1)">',
    'script tag': '<script>alert(1)</script>',
    'script inside SVG': '<svg><script>alert(1)</script></svg>',
    'event handler on SVG': '<svg onload="alert(1)"><circle r="5" /></svg>',
    'iframe': '<iframe src="https://evil.example"></iframe>',
    'object and embed': '<object data="evil.swf"></object><embed src="evil.swf">',
    'meta refresh': '<meta http-equiv="refresh" content="0;url=https://evil.example">',
    'base tag': '<base href="https://evil.example/">',
    'style tag': '<style>body { display: none }</style>',
    'style attribute': '<div style="position: fixed; inset: 0">fake dialog</div>',
    'form': '<form action="https://evil.example"><input name="password"><button>Log in</button></form>',
    'markdown javascript link': '[click me](javascript:alert(1))',
    'raw javascript link': '<a href="JaVaScRiPt:alert(1)">click me</a>',
    'data link': '<a href="data:text/html,<script>alert(1)</script>">click me</a>',
    'quotes in image alt text': '![x" onerror="alert(1)](a.png)',
    'quotes in image URL': '![x](a.png" onerror="alert(1))',
  }

  for (const [name, source] of Object.entries(attacks)) {
    it(name, () => {
      const container = render(source)
      for (const element of allElements(container)) {
        for (const attribute of element.attributes) {
          expect(attribute.name, `${element.tagName} has ${attribute.name}`).not.toMatch(/^on/i)
          expect(attribute.name, `${element.tagName} has a style attribute`).not.toBe('style')
          expect(attribute.value, `${element.tagName}[${attribute.name}]`).not.toMatch(/^\s*(javascript|data:text\/html)/i)
        }
      }
      const forbidden = container.querySelector('script, iframe, object, embed, meta, base, style, form, button, textarea, select')
      expect(forbidden?.outerHTML).toBeUndefined()
    })
  }
})

describe('renderMarkdown keeps ordinary Markdown', () => {
  it('formatting, lists, tables and links', () => {
    const container = render([
      '# Title',
      '',
      'Some **bold** and `code` and a [link](https://example.com).',
      '',
      '- one',
      '- two',
      '',
      '| a | b |',
      '| - | - |',
      '| 1 | 2 |',
    ].join('\n'))
    expect(container.querySelector('h1')?.textContent).toBe('Title')
    expect(container.querySelector('strong')?.textContent).toBe('bold')
    expect(container.querySelector('a')?.getAttribute('href')).toBe('https://example.com')
    expect(container.querySelectorAll('li')).toHaveLength(2)
    expect(container.querySelectorAll('td')).toHaveLength(2)
  })

  it('highlighted code blocks', () => {
    const container = render('```js\nconst answer = 42\n```')
    const code = container.querySelector('pre.hljs-md-pre code.hljs.language-js')
    expect(code?.textContent).toBe('const answer = 42')
    expect(code?.querySelector('span[class^="hljs-"]')).not.toBeNull()
  })

  it('task list checkboxes', () => {
    const checkbox = render('- [x] done').querySelector('input[type="checkbox"]')
    expect(checkbox?.hasAttribute('checked')).toBe(true)
  })

  it('safe raw HTML', () => {
    const container = render('<details><summary>More</summary>Hidden <kbd>Ctrl</kbd></details>')
    expect(container.querySelector('details summary')?.textContent).toBe('More')
    expect(container.querySelector('kbd')?.textContent).toBe('Ctrl')
  })

  const streamedPath = (source: string) => {
    const src = new URL(render(source).querySelector('img')!.getAttribute('src')!, 'http://localhost')
    expect(src.pathname).toBe('/api/v1/files/stream')
    expect([...src.searchParams.keys()]).toEqual(['path']) // no login token in URLs
    return src.searchParams.get('path')
  }

  it("images, relative to the Markdown file's folder", () => {
    expect(streamedPath('![x](img/a.png)')).toBe('notes/img/a.png')
    expect(streamedPath('![x](../shared/a.png)')).toBe('shared/a.png')
  })

  it('images with spaces and other encoded characters in their names', () => {
    expect(streamedPath('![x](img/a%20b.png)')).toBe('notes/img/a b.png')
    expect(streamedPath('![x](<img/a b.png>)')).toBe('notes/img/a b.png')
    expect(streamedPath('![x](img/%C3%BC.png)')).toBe('notes/img/ü.png')
    expect(streamedPath('![x](img/100%.png)')).toBe('notes/img/100%.png')
  })

  it('image alt text exactly as written', () => {
    const image = render('![A "quoted" & <odd> alt](a.png)').querySelector('img')!
    expect(image.getAttribute('alt')).toBe('A "quoted" & <odd> alt')
  })

  it('external images', () => {
    const image = render('![x](https://example.com/a.png?size=1&dark=1)').querySelector('img')!
    expect(image.getAttribute('src')).toBe('https://example.com/a.png?size=1&dark=1')
  })
})

describe('renderMarkdown line breaks', () => {
  const typed = 'first line\nsecond line\n\nnew paragraph'

  it('joins lines broken by a single line break into one paragraph, as Markdown files expect', () => {
    expect(renderMarkdown(typed, 'notes/readme.md')).toBe('<p>first line\nsecond line</p>\n<p>new paragraph</p>\n')
  })

  it('keeps every line break as typed with lineBreaks, for text written in a box', () => {
    expect(renderMarkdown(typed, 'board.kanban', { lineBreaks: true }))
      .toBe('<p>first line<br>second line</p>\n<p>new paragraph</p>\n')
  })
})
