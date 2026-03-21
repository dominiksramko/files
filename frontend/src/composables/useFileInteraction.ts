import { ref } from 'vue'
import type { FileEntry } from '@/types'
import { fileCategory, setMoveDragImage } from '@/utils/formatters'

const LONG_PRESS_DELAY_MS = 500

interface Emit {
  (event: 'toggleSelect', path: string, shift: boolean): void
  (event: 'navigate', path: string): void
  (event: 'preview', entry: FileEntry): void
  (event: 'archivePreview', entry: FileEntry): void
  (event: 'dropToFolder', destPath: string, paths: string[]): void
  (event: 'contextMenu', entry: FileEntry, x: number, y: number): void
}

export function useFileInteraction(props: { selected: string[] }, emit: Emit) {
  // ── Long press (mobile context menu) ─────────────────────────────────────
  let longPressTimer: ReturnType<typeof setTimeout> | null = null
  let longPressFired = false

  function onLongPressStart(entry: FileEntry, e: TouchEvent) {
    longPressFired = false
    const touch = e.touches[0]
    if (!touch) return
    const { clientX, clientY } = touch
    longPressTimer = setTimeout(() => {
      longPressTimer = null
      longPressFired = true
      emit('contextMenu', entry, clientX, clientY)
    }, LONG_PRESS_DELAY_MS)
  }

  function onLongPressEnd() {
    if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null }
  }

  // ── Click ────────────────────────────────────────────────────────────────
  function handleClick(entry: FileEntry, e: MouseEvent) {
    if (longPressFired) { longPressFired = false; return }
    if (props.selected.length > 0 || e.shiftKey) {
      emit('toggleSelect', entry.path, e.shiftKey)
      return
    }
    if (entry.is_dir) emit('navigate', entry.path)
    else if (fileCategory(entry.mime, entry.name) === 'archive') emit('archivePreview', entry)
    else emit('preview', entry)
  }

  // ── Drag & drop ──────────────────────────────────────────────────────────
  const dragOverPath = ref<string | null>(null)

  function onDragStart(e: DragEvent, entry: FileEntry) {
    const paths = props.selected.includes(entry.path) ? [...props.selected] : [entry.path]
    e.dataTransfer!.setData('application/x-files-internal', JSON.stringify(paths))
    e.dataTransfer!.effectAllowed = 'move'
    setMoveDragImage(e, paths.length)
  }

  function onDragEnd() {
    dragOverPath.value = null
  }

  function onDragOver(e: DragEvent, entry: FileEntry) {
    if (!entry.is_dir) return
    if (!e.dataTransfer?.types.includes('application/x-files-internal')) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    dragOverPath.value = entry.path
  }

  function onDragLeave(e: DragEvent, entry: FileEntry) {
    if (!entry.is_dir) return
    const related = e.relatedTarget as Node | null
    const el = e.currentTarget as HTMLElement
    if (!related || !el.contains(related)) {
      if (dragOverPath.value === entry.path) dragOverPath.value = null
    }
  }

  function onDrop(e: DragEvent, entry: FileEntry) {
    if (!entry.is_dir) return
    e.preventDefault()
    dragOverPath.value = null
    const raw = e.dataTransfer?.getData('application/x-files-internal')
    if (!raw) return
    const paths: string[] = JSON.parse(raw)
    emit('dropToFolder', entry.path, paths)
  }

  return { handleClick, dragOverPath, onDragStart, onDragEnd, onDragOver, onDragLeave, onDrop, onLongPressStart, onLongPressEnd }
}
