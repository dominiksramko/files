// Paths in the user's folder: relative, '/'-separated, '' for the folder itself.

// The last part of path: the name of the file or folder it leads to.
export function nameOf(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

// The folder holding path ('' for something in the home folder).
export function parentOf(path: string): string {
  const slash = path.lastIndexOf('/')
  return slash < 0 ? '' : path.slice(0, slash)
}

// Whether path is the folder root or inside it.
export function isWithin(root: string, path: string): boolean {
  return root === '' || path === root || path.startsWith(root + '/')
}

// path as seen from root ('' for root itself); path must be within root.
export function relativeTo(root: string, path: string): string {
  if (path === root) return ''
  return root === '' ? path : path.slice(root.length + 1)
}

export interface Crumb {
  label: string
  path: string
  isHome: boolean
}

// The breadcrumb trail to path, starting at root (labelled rootLabel if given,
// else Home when it's the home folder, else its name).
export function breadcrumbTrail(path: string, root = '', rootLabel?: string): Crumb[] {
  const crumbs: Crumb[] = [{ label: rootLabel ?? (root ? nameOf(root) : 'Home'), path: root, isHome: true }]
  const below = isWithin(root, path) ? relativeTo(root, path) : ''
  below
    .split('/')
    .filter(Boolean)
    .forEach((segment, index, segments) => {
      crumbs.push({ label: segment, path: [root, ...segments.slice(0, index + 1)].filter(Boolean).join('/'), isHome: false })
    })
  return crumbs
}

// The path link leads to from folder: relative ("img/a.png", "../b.pdf"), or
// from the home folder when it starts with "/". null when it climbs above the
// home folder, where nothing can be reached.
export function resolvePath(folder: string, link: string): string | null {
  const segments = link.startsWith('/') ? [] : folder.split('/').filter(Boolean)
  for (const segment of link.split('/')) {
    if (segment === '' || segment === '.') continue
    if (segment !== '..') segments.push(segment)
    else if (segments.pop() === undefined) return null
  }
  return segments.join('/')
}

// How to reach path from folder: the link resolvePath(folder, link) turns back into path.
export function relativePath(folder: string, path: string): string {
  const from = folder.split('/').filter(Boolean)
  const to = path.split('/').filter(Boolean)
  let shared = 0
  while (shared < from.length && shared < to.length - 1 && from[shared] === to[shared]) shared++
  return [...Array<string>(from.length - shared).fill('..'), ...to.slice(shared)].join('/')
}
