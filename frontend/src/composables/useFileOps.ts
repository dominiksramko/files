import { ref, computed } from 'vue'
import type { Ref, ComputedRef } from 'vue'
import type { FileEntry, FileOp } from '@/types'
import { fileops, ConflictError } from '@/utils/api'
import type { ContextMenuItem } from '@/components/ui/ContextMenu.vue'

export interface FileOpsActionCallbacks {
  downloadPaths: (paths: string[]) => void
  openCompressDialog: (paths: string[]) => void
  openExtract: (path: string) => void
  startRename: (entry: FileEntry) => void
  openAdvancedRenamer: (paths: string[]) => void
  openDeleteDialog: (paths: string[]) => void
  openDetails: (entries: FileEntry[]) => void
  entriesForPaths: (paths: string[]) => FileEntry[]
}

export function useFileOps({
  selectedPaths,
  visibleEntries,
  searchResults,
  downloading,
  currentPath,
  promptConflict,
  showToast,
  actionCallbacks,
}: {
  selectedPaths: Ref<string[]>
  visibleEntries: ComputedRef<FileEntry[]>
  searchResults: Ref<FileEntry[]>
  downloading: ComputedRef<boolean>
  currentPath: ComputedRef<string>
  promptConflict: (title: string, names: string[]) => Promise<'replace' | 'rename' | null>
  showToast: (msg: string, type: 'success' | 'error' | 'info') => void
  actionCallbacks: FileOpsActionCallbacks
}) {
  // ── Clipboard ───────────────────────────────────────────────────────────────
  const clipboard = ref<{ mode: 'copy' | 'move'; paths: string[] } | null>(null)

  // ── Context menu state ──────────────────────────────────────────────────────
  const contextMenuState = ref<{ x: number; y: number; entry: FileEntry; paths: string[] } | null>(null)

  function onContextMenu(entry: FileEntry, x: number, y: number) {
    const paths = selectedPaths.value.includes(entry.path)
      ? [...selectedPaths.value]
      : [entry.path]
    contextMenuState.value = { x, y, entry, paths }
  }

  // ── Build file ops ──────────────────────────────────────────────────────────
  // Single source of truth for all file operations. Both the toolbar (fileOps)
  // and context menu (contextMenuItems) derive from this.
  function buildFileOps(paths: string[], entry: FileEntry | null, after: () => void = () => {}): FileOp[] {
    const single = paths.length === 1
    return [
      {
        key: 'copy', label: 'Copy', icon: 'copy',
        action: () => { clipboard.value = { mode: 'copy', paths }; selectedPaths.value = []; after() },
      },
      {
        key: 'move', label: 'Move', icon: 'arrowRight',
        action: () => { clipboard.value = { mode: 'move', paths }; selectedPaths.value = selectedPaths.value.filter(p => !paths.includes(p)); after() },
      },
      {
        key: 'download', label: 'Download', icon: 'download', disabled: downloading.value,
        action: () => { after(); actionCallbacks.downloadPaths(paths) },
      },
      {
        key: 'compress', label: 'Compress', icon: 'archive',
        action: () => { after(); actionCallbacks.openCompressDialog(paths) },
      },
      {
        key: 'extract', label: 'Extract here', icon: 'archiveOut',
        visible: single && !!entry?.can_extract,
        action: () => { after(); actionCallbacks.openExtract(entry!.path) },
      },
      {
        key: 'rename', label: 'Rename', icon: 'pencil',
        visible: single,
        action: () => { after(); if (entry) actionCallbacks.startRename(entry) },
      },
      {
        key: 'rename-advanced', label: 'Rename (advanced)', icon: 'pencilSquare',
        action: () => { after(); actionCallbacks.openAdvancedRenamer(paths) },
      },
      {
        key: 'delete', label: 'Delete', icon: 'trash', danger: true,
        action: () => { after(); actionCallbacks.openDeleteDialog(paths) },
      },
      {
        key: 'details', label: 'Details', icon: 'info',
        action: () => { after(); actionCallbacks.openDetails(actionCallbacks.entriesForPaths(paths)) },
      },
    ]
  }

  // ── Toolbar ops (selection-driven) ──────────────────────────────────────────
  const fileOps = computed((): FileOp[] => {
    if (selectedPaths.value.length === 0) return []
    const paths = [...selectedPaths.value]
    const pool = [...visibleEntries.value, ...searchResults.value]
    const entry = paths.length === 1 ? (pool.find(e => e.path === paths[0]) ?? null) : null
    return buildFileOps(paths, entry)
  })

  // ── Context menu items ──────────────────────────────────────────────────────
  const contextMenuItems = computed((): ContextMenuItem[] => {
    if (!contextMenuState.value) return []
    const { entry, paths } = contextMenuState.value
    const close = () => { contextMenuState.value = null }
    const ops = buildFileOps(paths, paths.length === 1 ? entry : null, close)
    const get = (key: string) => ops.find(op => op.key === key)!
    const toItem = (op: FileOp): ContextMenuItem => ({ label: op.label, icon: op.icon, danger: op.danger, action: op.action })
    const items: ContextMenuItem[] = [
      toItem(get('copy')),
      toItem(get('move')),
      toItem(get('download')),
      toItem(get('compress')),
      { separator: true },
    ]
    if (get('extract').visible !== false) items.push(toItem(get('extract')))
    if (get('rename').visible !== false) items.push(toItem(get('rename')))
    items.push(toItem(get('rename-advanced')))
    items.push(toItem(get('delete')))
    items.push({ separator: true })
    items.push(toItem(get('details')))
    return items
  })

  // ── Paste (copy/move from clipboard) ───────────────────────────────────────
  async function doPaste(mode: 'copy' | 'move', paths: string[], dest: string, conflictMode: 'error' | 'replace' | 'rename' = 'error') {
    try {
      if (mode === 'copy') {
        await fileops.copy(paths, dest, conflictMode)
      } else {
        await fileops.move(paths, dest, conflictMode)
        selectedPaths.value = selectedPaths.value.filter((p) => !paths.includes(p))
      }
      clipboard.value = null
    } catch (err) {
      if (err instanceof ConflictError) {
        const label = mode === 'copy' ? 'Copy' : 'Move'
        const choice = await promptConflict(`${label} conflicts`, err.conflicts)
        if (choice) doPaste(mode, paths, dest, choice)
        return
      }
      showToast((err as Error).message, 'error')
    }
  }

  async function onPaste() {
    if (!clipboard.value) return
    const { mode, paths } = clipboard.value
    await doPaste(mode, paths, currentPath.value)
  }

  // ── Internal drag-and-drop move ─────────────────────────────────────────────
  async function performMove(paths: string[], dest: string, conflictMode: 'error' | 'replace' | 'rename' = 'error') {
    try {
      await fileops.move([...paths], dest, conflictMode)
      selectedPaths.value = selectedPaths.value.filter((p) => !paths.includes(p))
      const count = paths.length
      showToast(`Moved ${count} item${count !== 1 ? 's' : ''}`, 'success')
    } catch (err) {
      if (err instanceof ConflictError) {
        const choice = await promptConflict('Move conflicts', err.conflicts)
        if (choice) performMove(paths, dest, choice)
        return
      }
      showToast((err as Error).message, 'error')
    }
  }

  return {
    clipboard,
    contextMenuState,
    onContextMenu,
    fileOps,
    contextMenuItems,
    onPaste,
    doPaste,
    performMove,
  }
}
