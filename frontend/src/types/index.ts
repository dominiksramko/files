export interface User {
  id: number
  username: string
  root_dir: string
  is_admin: boolean
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
