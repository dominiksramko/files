import type { FileCategory } from '@/types'

export function formatSize(bytes: number): string {
  if (bytes === 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return (bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const TEXT_EXTS = new Set([
  'txt', 'log', 'md', 'mdx', 'markdown', 'rst', 'csv', 'tsv',
  'json', 'jsonc', 'json5', 'yaml', 'yml', 'toml', 'ini', 'conf', 'cfg', 'env',
  'xml', 'svg', 'html', 'htm', 'xhtml',
  'js', 'cjs', 'mjs', 'ts', 'tsx', 'jsx', 'vue', 'svelte',
  'css', 'scss', 'sass', 'less',
  'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'cmd',
  'py', 'pyw', 'rb', 'php', 'java', 'c', 'cpp', 'cc', 'h', 'hpp',
  'cs', 'go', 'rs', 'kt', 'kts', 'swift', 'dart', 'lua',
  'r', 'sql', 'graphql', 'gql',
  'dockerfile', 'makefile', 'rakefile', 'gemfile', 'procfile',
  'gitignore', 'dockerignore', 'editorconfig', 'gitattributes',
  'htaccess', 'nginx', 'vim', 'el', 'clj', 'scala', 'ex', 'exs', 'erl',
  'tf', 'hcl', 'nix', 'zig',
])

export function isTextFile(mime: string, name: string): boolean {
  if (mime.startsWith('text/')) return true
  if (['application/json', 'application/xml', 'application/javascript',
       'application/typescript', 'application/x-sh', 'application/x-httpd-php',
       'application/graphql'].includes(mime)) return true
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  return TEXT_EXTS.has(ext)
}

export function isMarkdownFile(mime: string, name: string): boolean {
  if (mime === 'text/markdown' || mime === 'text/x-markdown') return true
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  return ext === 'md' || ext === 'mdx' || ext === 'markdown'
}

export function fileCategory(mime: string, name: string): FileCategory {
  if (!mime || mime === 'application/octet-stream') {
    // Fall back to extension
    const ext = name.split('.').pop()?.toLowerCase() ?? ''
    const archiveExts = ['zip', 'tar', 'gz', 'tgz', 'bz2', 'xz', 'rar', '7z', 'zst']
    if (archiveExts.includes(ext)) return 'archive'
    if (TEXT_EXTS.has(ext)) return 'text'
    return 'other'
  }

  if (mime.startsWith('image/')) return 'image'
  if (mime.startsWith('video/')) return 'video'
  if (mime.startsWith('audio/')) return 'audio'
  if (mime === 'application/pdf') return 'pdf'
  if (
    mime.includes('word') ||
    mime.includes('excel') ||
    mime.includes('powerpoint') ||
    mime.includes('spreadsheet') ||
    mime.includes('presentation') ||
    mime.includes('opendocument')
  )
    return 'office'
  if (
    mime === 'application/zip' ||
    mime === 'application/x-tar' ||
    mime === 'application/gzip' ||
    mime === 'application/x-bzip2' ||
    mime === 'application/x-xz' ||
    mime === 'application/x-rar-compressed' ||
    mime === 'application/x-7z-compressed'
  )
    return 'archive'
  if (isTextFile(mime, name)) return 'text'
  return 'other'
}

export function isPreviewable(mime: string): boolean {
  if (!mime) return false
  return (
    mime.startsWith('image/') ||
    mime.startsWith('video/') ||
    mime.startsWith('audio/') ||
    mime === 'application/pdf'
  )
}
