import DOMPurify from 'dompurify'
import { Marked, type Tokens } from 'marked'
import hljs from 'highlight.js/lib/common'
import { files as filesApi } from '@/utils/api'
import { escapeHtml } from '@/utils/html'
import { parentOf } from '@/utils/paths'

// Relative image paths in a Markdown file resolve against that file's folder,
// so `![](img/a.png)` in `notes/readme.md` streams `notes/img/a.png`.
function resolveMarkdownImageSrc(entryPath: string, href: string): string {
  if (/^https?:\/\//.test(href) || href.startsWith('/')) return href
  const dir = parentOf(entryPath)
  // The URL keeps names percent-encoded (`a%20b.png`); the API wants the real name.
  const encodedPath = new URL(href, `http://x/${dir}/`).pathname.replace(/^\//, '')
  let resolvedPath = encodedPath
  try {
    resolvedPath = decodeURIComponent(encodedPath)
  } catch {
    // A stray "%" that isn't an escape: the name is meant literally.
  }
  return filesApi.streamUrlByPath(resolvedPath)
}

// Markdown may contain raw HTML, and the result goes into the page with v-html,
// so it's sanitised: no scripts, event handlers or javascript: links, which
// would run with the user's session. Styles and forms are removed too, so a file
// can't lay a fake dialog or login form over the app.
const SANITIZE_OPTIONS = {
  FORBID_TAGS: ['style', 'form', 'button', 'textarea', 'select'],
  FORBID_ATTR: ['style'],
}

// Renders Markdown to safe HTML with highlighted code blocks. entryPath is the
// file the Markdown belongs to; each call gets its own parser so concurrent
// previews never resolve images against another file's folder.
// lineBreaks keeps every line break as typed, for text written in a box (a
// card's description); without it a single line break joins the lines into one
// paragraph, as Markdown files expect (many are wrapped at a fixed width).
export function renderMarkdown(source: string, entryPath: string, { lineBreaks = false } = {}): string {
  const parser = new Marked({
    breaks: lineBreaks,
    renderer: {
      code({ text, lang }: Tokens.Code) {
        const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
        const highlighted = hljs.highlight(text, { language }).value
        return `<pre class="hljs-md-pre"><code class="hljs language-${language}">${highlighted}</code></pre>`
      },
      image({ href, text }: Tokens.Image) {
        return `<img src="${escapeHtml(resolveMarkdownImageSrc(entryPath, href))}" alt="${escapeHtml(text)}">`
      },
    },
  })
  return DOMPurify.sanitize(parser.parse(source, { async: false }), SANITIZE_OPTIONS)
}
