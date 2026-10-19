import { Marked, type Tokens } from 'marked'
import hljs from 'highlight.js/lib/common'
import { files as filesApi } from '@/utils/api'

// Relative image paths in a Markdown file resolve against that file's folder,
// so `![](img/a.png)` in `notes/readme.md` streams `notes/img/a.png`.
function resolveMarkdownImageSrc(entryPath: string, href: string): string {
  if (/^https?:\/\//.test(href) || href.startsWith('/')) return href
  const dir = entryPath.substring(0, entryPath.lastIndexOf('/'))
  const resolvedPath = new URL(href, `http://x/${dir}/`).pathname.replace(/^\//, '')
  return filesApi.streamUrlByPath(resolvedPath)
}

// Renders Markdown to HTML with highlighted code blocks. entryPath is the file
// the Markdown belongs to; each call gets its own parser so concurrent previews
// never resolve images against another file's folder.
export function renderMarkdown(source: string, entryPath: string): string {
  const parser = new Marked({
    renderer: {
      code({ text, lang }: Tokens.Code) {
        const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
        const highlighted = hljs.highlight(text, { language }).value
        return `<pre class="hljs-md-pre"><code class="hljs language-${language}">${highlighted}</code></pre>`
      },
      image({ href, text }: Tokens.Image) {
        return `<img src="${resolveMarkdownImageSrc(entryPath, href)}" alt="${text}">`
      },
    },
  })
  return parser.parse(source, { async: false })
}
