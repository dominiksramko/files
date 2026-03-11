import type { FileCategory } from '@/types'
import type { IconName } from '@/utils/icons'

// Creates a compact drag ghost ("Moving 3 items") and sets it as the drag image.
// The element is appended off-screen, used for one frame, then removed.
export function setMoveDragImage(e: DragEvent, count: number): void {
  const ghost = document.createElement('div')
  ghost.textContent = `Moving ${count} item${count !== 1 ? 's' : ''}`
  Object.assign(ghost.style, {
    position: 'fixed',
    top: '-1000px',
    left: '-1000px',
    background: '#27272a',
    border: '1px solid #3f3f46',
    borderRadius: '8px',
    padding: '6px 12px',
    color: '#f4f4f5',
    fontSize: '13px',
    fontFamily: 'inherit',
    whiteSpace: 'nowrap',
    pointerEvents: 'none',
  })
  document.body.appendChild(ghost)
  e.dataTransfer!.setDragImage(ghost, -12, -12)
  requestAnimationFrame(() => ghost.remove())
}

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

const OFFICE_MIMES = new Set([
  'application/msword',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.ms-excel',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.ms-powerpoint',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  'application/vnd.oasis.opendocument.text',
  'application/vnd.oasis.opendocument.spreadsheet',
  'application/vnd.oasis.opendocument.presentation',
])

const OFFICE_EXTS = new Set([
  'doc', 'dot', 'docx', 'docm', 'dotx', 'dotm',
  'xls', 'xlt', 'xlsx', 'xlsm', 'xlsb', 'xltx', 'xltm',
  'ppt', 'pot', 'pps', 'pptx', 'pptm', 'potx', 'potm', 'ppsx', 'ppsm',
  'odt', 'ott', 'ods', 'ots', 'odp', 'otp',
])

export function isOfficeFile(mime: string, name: string): boolean {
  if (OFFICE_MIMES.has(mime)) return true
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  return OFFICE_EXTS.has(ext)
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

export type EntryCat = FileCategory | 'folder'

export function entryCategory(mime: string, name: string, isDir: boolean): EntryCat {
  return isDir ? 'folder' : fileCategory(mime, name)
}

export const catBg: Record<EntryCat, string> = {
  folder:  'bg-yellow-950/60',
  image:   'bg-green-950/60',
  video:   'bg-purple-950/60',
  audio:   'bg-pink-950/60',
  pdf:     'bg-red-950/60',
  office:  'bg-blue-950/60',
  archive: 'bg-orange-950/60',
  text:    'bg-zinc-800/60',
  other:   'bg-zinc-800/60',
}

export const catIconColor: Record<EntryCat, string> = {
  folder:  'text-yellow-400',
  image:   'text-green-400',
  video:   'text-purple-400',
  audio:   'text-pink-400',
  pdf:     'text-red-400',
  office:  'text-blue-400',
  archive: 'text-orange-400',
  text:    'text-zinc-300',
  other:   'text-zinc-500',
}

export const catIconName: Record<EntryCat, IconName> = {
  folder:  'folder',
  image:   'image',
  video:   'video',
  audio:   'music',
  pdf:     'pdf',
  office:  'document',
  archive: 'archive',
  text:    'document',
  other:   'document',
}

export function fileExtension(name: string): string {
  const parts = name.split('.')
  if (parts.length >= 3 && parts[parts.length - 2]?.toLowerCase() === 'tar') {
    return `TAR.${parts[parts.length - 1]!.toUpperCase()}`
  }
  const ext = parts.length >= 2 ? parts[parts.length - 1] : undefined
  return (ext ?? '').toUpperCase()
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
