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
  token: string
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
  can_extract?: boolean
}

export interface ArchiveEntry {
  name: string
  size: number
  is_dir: boolean
}

export type FileCategory =
  | 'image'
  | 'video'
  | 'audio'
  | 'pdf'
  | 'office'
  | 'archive'
  | 'text'
  | 'other'
