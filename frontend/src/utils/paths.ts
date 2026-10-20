// Paths in the user's folder: relative, '/'-separated, '' for the folder itself.

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

// The breadcrumb trail to path, starting at root (labelled Home when it's the
// home folder).
export function breadcrumbTrail(path: string, root = ''): Crumb[] {
  const crumbs: Crumb[] = [{ label: root ? root.split('/').pop()! : 'Home', path: root, isHome: true }]
  const below = isWithin(root, path) ? relativeTo(root, path) : ''
  below
    .split('/')
    .filter(Boolean)
    .forEach((segment, index, segments) => {
      crumbs.push({ label: segment, path: [root, ...segments.slice(0, index + 1)].filter(Boolean).join('/'), isHome: false })
    })
  return crumbs
}
