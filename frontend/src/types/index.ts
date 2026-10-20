export interface SubtitlePrefs {
  color: string
  bg_opacity: number
  font_size: number
  text_style: string
  font_family: string
}

export interface User {
  id: number
  username: string
  display_name?: string
  root_dir: string
  is_admin: boolean
  subtitle_prefs?: SubtitlePrefs
}

export interface LoginResponse {
  user: User
}

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mod_time: string
  created_at?: string
  mime: string
  has_thumb: boolean
  thumb_hash?: string
  content_hash?: string
  can_extract?: boolean
}

// What a folder (or file) takes, from a disk usage scan: disk is the space
// taken on disk, with a file's hard links counted once.
export interface UsageTotals {
  files: number
  dirs: number
  disk: number
}

// An item of the folder shown on the disk usage page. A folder that isn't
// scanned (made since the scan, or a link) carries only its own size.
// hard_links: a file's number of names when it has several (they share its
// space, which the scan's totals count once).
export interface UsageEntry extends FileEntry, UsageTotals {
  link?: boolean
  hard_links?: number
  scanned: boolean
}

export interface UsageListing {
  root: string
  at: string
  scanned_at: string
  unreadable: number
  root_total: UsageTotals
  total: UsageTotals
  entries: UsageEntry[]
}

export interface ArchiveEntry {
  name: string
  size: number
  is_dir: boolean
}

import type { IconName } from '@/utils/icons'

export interface FileOp {
  key: string
  label: string
  icon: IconName
  danger?: boolean
  visible?: boolean
  disabled?: boolean
  action: () => void
}

export interface RenamePreset {
  id: number
  name: string
  template: string
  created_at: string
}

export interface RenamePreviewItem {
  original_name: string
  new_name: string
  error?: string
}

export type FileCategory =
  | 'image'
  | 'video'
  | 'audio'
  | 'pdf'
  | 'office'
  | 'archive'
  | 'text'
  | 'todo'
  | 'kanban'
  | 'other'
