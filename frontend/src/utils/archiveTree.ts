import type { LocationQuery } from 'vue-router'
import type { ArchiveEntry } from '@/types'
import { isWithin, nameOf, parentOf } from '@/utils/paths'
import { queryString, withKeys } from '@/utils/query'

// A file or folder in an archive, at its path inside it ('/'-separated). A
// folder's size is the total of the files under it; encrypted marks a file
// that needs a password to extract.
export interface ArchiveItem {
  name: string
  path: string
  is_dir: boolean
  size: number
  encrypted: boolean
}

export interface ArchiveTree {
  // Every file and folder in the archive.
  items: ArchiveItem[]
  // What each folder holds ('' for the top of the archive), folders first.
  folders: Map<string, ArchiveItem[]>
  // Whether any file needs a password to extract.
  encrypted: boolean
}

// An entry's path as the formats write it, made plain: tar starts names with
// './', folders end in '/'.
export function archivePath(name: string): string {
  return name.split('/').filter((segment) => segment !== '' && segment !== '.').join('/')
}

// Folders first, then by name, numbers in order (2 before 10).
export function compareArchiveItems(first: ArchiveItem, second: ArchiveItem): number {
  if (first.is_dir !== second.is_dir) return first.is_dir ? -1 : 1
  return first.name.localeCompare(second.name, undefined, { numeric: true, sensitivity: 'base' })
}

// The archive as folders. Archives needn't list a folder before (or at all)
// what's in it, so folders are made from the paths of what they hold.
export function archiveTree(entries: ArchiveEntry[]): ArchiveTree {
  const byPath = new Map<string, ArchiveItem>()

  function folderAt(path: string): ArchiveItem {
    let folder = byPath.get(path)
    if (!folder?.is_dir) {
      folder = { name: nameOf(path), path, is_dir: true, size: 0, encrypted: false }
      byPath.set(path, folder)
    }
    return folder
  }

  function addToFolders(path: string, bytes: number) {
    for (let folder = parentOf(path); folder !== ''; folder = parentOf(folder)) folderAt(folder).size += bytes
  }

  for (const entry of entries) {
    const path = archivePath(entry.name)
    if (!path) continue
    if (parentOf(path)) folderAt(parentOf(path))
    if (entry.is_dir) {
      folderAt(path)
      continue
    }
    // An archive can hold the same file more than once (tar appends); the
    // last one is what extracting leaves.
    const earlier = byPath.get(path)
    if (earlier?.is_dir) continue
    addToFolders(path, entry.size - (earlier?.size ?? 0))
    byPath.set(path, { name: nameOf(path), path, is_dir: false, size: entry.size, encrypted: entry.encrypted === true })
  }

  const items = [...byPath.values()]
  const folders = new Map<string, ArchiveItem[]>([['', []]])
  for (const item of items) {
    if (item.is_dir && !folders.has(item.path)) folders.set(item.path, [])
    const parent = parentOf(item.path)
    if (!folders.has(parent)) folders.set(parent, [])
    folders.get(parent)!.push(item)
  }
  for (const contents of folders.values()) contents.sort(compareArchiveItems)
  return { items, folders, encrypted: items.some((item) => item.encrypted) }
}

// What's under folder (at any depth) with query in its name, ignoring case.
export function searchArchive(items: ArchiveItem[], folder: string, query: string): ArchiveItem[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return []
  return items
    .filter((item) => item.path !== folder && isWithin(folder, item.path) && item.name.toLowerCase().includes(needle))
    .sort((first, second) => compareArchiveItems(first, second) || first.path.localeCompare(second.path))
}

// What the archive modal shows: the archive at path (in the user's folder) and
// the folder at inside it ('' for its top).
export interface ArchiveState {
  path: string
  at: string
}

// The modal's keys in the file browser's URL: ?archive= and ?archive_at=.
const archiveKeys = ['archive', 'archive_at']

// The modal's state in query, or null when it isn't open.
export function archiveStateFrom(query: LocationQuery): ArchiveState | null {
  const path = queryString(query.archive)
  if (!path) return null
  return { path, at: archivePath(queryString(query.archive_at) ?? '') }
}

// query with the modal showing state (null closes it); the file browser's own
// keys stay.
export function withArchiveState(query: LocationQuery, state: ArchiveState | null): LocationQuery {
  return withKeys(query, archiveKeys, { archive: state?.path, archive_at: state?.at || undefined })
}
