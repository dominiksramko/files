<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useFilesStore } from '@/stores/files'
import type { FileEntry } from '@/types'
import { fileCategory, formatSize, formatDate } from '@/utils/formatters'
import { files as filesApi, folders as foldersApi, fileops } from '@/utils/api'
import { useThumbnails } from '@/composables/useThumbnails'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import FileList from '@/components/files/FileList.vue'
import FileGrid from '@/components/files/FileGrid.vue'
import FileSearch from '@/components/files/FileSearch.vue'
import FilePreview from '@/components/files/FilePreview.vue'
import ArchiveContents from '@/components/archive/ArchiveContents.vue'
import ContextMenu from '@/components/ui/ContextMenu.vue'
import type { ContextMenuItem } from '@/components/ui/ContextMenu.vue'

const filesStore = useFilesStore()
const route = useRoute()
const router = useRouter()

// ── Thumbnails ───────────────────────────────────────────────────────────────
const { connect: thumbConnect } = useThumbnails()

// ── View mode (per-folder, stored on server) ────────────────────────────────
const viewMode = ref<'grid' | 'list'>('list')
const prefLoading = ref(false)

async function fetchFolderPref(path: string) {
  prefLoading.value = true
  try {
    const { view_mode } = await foldersApi.getPref(path)
    if (filesStore.currentPath === path) viewMode.value = view_mode
  } catch {
    // keep current viewMode on error
  } finally {
    prefLoading.value = false
  }
}

async function setView(mode: 'grid' | 'list') {
  viewMode.value = mode
  try {
    await foldersApi.setPref(filesStore.currentPath, mode)
  } catch {
    // ignore — pref will re-auto-detect on next visit
  }
}

// ── Hidden files ────────────────────────────────────────────────────────────
const showHidden = ref(localStorage.getItem('showHidden') === 'true')
function toggleHidden() {
  showHidden.value = !showHidden.value
  localStorage.setItem('showHidden', String(showHidden.value))
}

// ── Search ──────────────────────────────────────────────────────────────────
const searchQuery = ref('')
const searchResults = ref<FileEntry[]>([])
const searchLoading = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

watch(searchQuery, (query) => {
  if (((route.query.q as string) ?? '') !== query) {
    router.replace({ query: routeQuery() })
  }
  if (searchTimer) clearTimeout(searchTimer)
  if (!query.trim()) {
    searchResults.value = []
    searchLoading.value = false
    return
  }
  searchLoading.value = true
  searchTimer = setTimeout(async () => {
    try {
      searchResults.value = await filesApi.search(query.trim())
    } catch {
      searchResults.value = []
    } finally {
      searchLoading.value = false
    }
  }, 300)
})

function clearSearch() {
  searchQuery.value = ''
  searchResults.value = []
}

// ── Sort state ─────────────────────────────────────────────────────────────
type SortKey = 'name' | 'size' | 'mod_time'
const sortKey = ref<SortKey>('name')
const sortDesc = ref(false)

function toggleSort(key: string) {
  if (sortKey.value === key) {
    sortDesc.value = !sortDesc.value
  } else {
    sortKey.value = key as SortKey
    sortDesc.value = false
  }
}

const sortedEntries = computed(() => {
  const arr = [...filesStore.entries]
  arr.sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    let cmp = 0
    if (sortKey.value === 'name') cmp = a.name.localeCompare(b.name)
    else if (sortKey.value === 'size') cmp = a.size - b.size
    else cmp = new Date(a.mod_time).getTime() - new Date(b.mod_time).getTime()
    return sortDesc.value ? -cmp : cmp
  })
  return arr
})

const visibleEntries = computed(() =>
  showHidden.value ? sortedEntries.value : sortedEntries.value.filter((e) => !e.name.startsWith('.')),
)

// ── Preview navigation ─────────────────────────────────────────────────────
const previewIndex = ref(-1)
const archiveEntry = ref<FileEntry | null>(null)

const previewableEntries = computed(() => {
  const pool = searchQuery.value ? searchResults.value : visibleEntries.value
  return pool.filter((e) => !e.is_dir && fileCategory(e.mime, e.name) !== 'archive')
})

const previewEntry = computed<FileEntry | null>(
  () => previewableEntries.value[previewIndex.value] ?? null,
)

function openPreview(entry: FileEntry) {
  previewIndex.value = previewableEntries.value.findIndex((e) => e.path === entry.path)
}

function closePreview() { previewIndex.value = -1 }
function prevPreview() { if (previewIndex.value > 0) previewIndex.value-- }
function nextPreview() {
  if (previewIndex.value < previewableEntries.value.length - 1) previewIndex.value++
}

// ── URL-synced navigation ──────────────────────────────────────────────────
function routeQuery() {
  const q: Record<string, string> = {}
  if (filesStore.currentPath) q.path = filesStore.currentPath
  if (searchQuery.value) q.q = searchQuery.value
  if (previewEntry.value) q.preview = previewEntry.value.path
  return q
}

function navigate(path: string) {
  thumbConnect(path)
  filesStore.navigate(path)
  fetchFolderPref(path)
  selectedPaths.value = []
  router.push({ query: path ? { path } : {} })
}

watch(previewEntry, (entry) => {
  const currentPreview = route.query.preview as string | undefined
  if (entry?.path === currentPreview) return
  router.replace({ query: routeQuery() })
})

watch(
  () => route.query,
  (q) => {
    const p = (q.path as string) || ''
    if (p !== filesStore.currentPath) {
      thumbConnect(p)
      filesStore.navigate(p)
      fetchFolderPref(p)
      selectedPaths.value = []
    }
    const qParam = (q.q as string) ?? ''
    if (searchQuery.value !== qParam) searchQuery.value = qParam

    const previewPath = q.preview as string | undefined
    if (previewEntry.value?.path === previewPath) return
    if (!previewPath) { previewIndex.value = -1; return }
    const idx = previewableEntries.value.findIndex((e) => e.path === previewPath)
    previewIndex.value = idx >= 0 ? idx : -1
  },
)

watch(previewableEntries, (entries) => {
  const previewPath = route.query.preview as string | undefined
  if (!previewPath || previewIndex.value >= 0) return
  previewIndex.value = entries.findIndex((e) => e.path === previewPath)
})

// ── Archive ────────────────────────────────────────────────────────────────
function onExtracted() {
  archiveEntry.value = null
  filesStore.refresh()
}

// ── Selection ──────────────────────────────────────────────────────────────
const selectedPaths = ref<string[]>([])

function toggleSelect(path: string) {
  const idx = selectedPaths.value.indexOf(path)
  if (idx >= 0) selectedPaths.value.splice(idx, 1)
  else selectedPaths.value.push(path)
}

function selectAll() {
  selectedPaths.value = visibleEntries.value.map((e) => e.path)
}

function deselectAll() {
  selectedPaths.value = []
}

// ── Clipboard ──────────────────────────────────────────────────────────────
const clipboard = ref<{ mode: 'copy' | 'move'; paths: string[] } | null>(null)

function onCopy() {
  clipboard.value = { mode: 'copy', paths: [...selectedPaths.value] }
}

function onMove() {
  clipboard.value = { mode: 'move', paths: [...selectedPaths.value] }
  selectedPaths.value = []
}

async function onPaste() {
  if (!clipboard.value) return
  const { mode, paths } = clipboard.value
  try {
    if (mode === 'copy') {
      await fileops.copy(paths, filesStore.currentPath)
    } else {
      await fileops.move(paths, filesStore.currentPath)
    }
    clipboard.value = null
    filesStore.refresh()
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

// ── Download selection ─────────────────────────────────────────────────────
const downloading = ref(false)

async function downloadPaths(paths: string[]) {
  if (paths.length === 1) {
    const pool = [...visibleEntries.value, ...searchResults.value]
    const entry = pool.find((e) => e.path === paths[0])
    if (entry && !entry.is_dir) {
      const a = document.createElement('a')
      a.href = filesApi.downloadUrl(entry.path, entry.mod_time)
      a.click()
      return
    }
  }
  downloading.value = true
  try {
    const blob = await fileops.downloadZip(paths)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const pool = [...visibleEntries.value, ...searchResults.value]
    const single = paths.length === 1 ? pool.find((e) => e.path === paths[0]) : null
    const folderName = filesStore.currentPath.split('/').filter(Boolean).pop() ?? 'download'
    a.download = single ? `${single.name}.zip` : `${folderName}.zip`
    a.click()
    URL.revokeObjectURL(url)
  } catch (err) {
    showToast((err as Error).message, 'error')
  } finally {
    downloading.value = false
  }
}

function onDownload() {
  downloadPaths([...selectedPaths.value])
}

// ── Delete modal (unified for toolbar selection + preview) ─────────────────
const deleteDialog = ref(false)
const deletePending = ref<{ paths: string[]; name?: string } | null>(null)

function openDeleteDialog(paths: string[], name?: string) {
  const resolvedName = name ?? (paths.length === 1
    ? (filesStore.entries.find(e => e.path === paths[0])?.name
      ?? searchResults.value.find(e => e.path === paths[0])?.name)
    : undefined)
  deletePending.value = { paths: [...paths], name: resolvedName }
  deleteDialog.value = true
}

async function confirmDelete() {
  if (!deletePending.value) return
  const { paths } = deletePending.value
  deleteDialog.value = false
  deletePending.value = null
  try {
    await fileops.delete(paths)
    if (paths.some(p => previewEntry.value?.path === p)) closePreview()
    selectedPaths.value = selectedPaths.value.filter(p => !paths.includes(p))
    filesStore.refresh()
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

function cancelDelete() {
  deleteDialog.value = false
  deletePending.value = null
}

// ── Upload ─────────────────────────────────────────────────────────────────
const uploadInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)

// ── Upload conflict modal ──────────────────────────────────────────────────
const uploadConflictDialog = ref(false)
const pendingUploadFiles = ref<File[]>([])
const conflictingNames = ref<string[]>([])

function detectConflicts(files: File[]): string[] {
  const existing = new Set(filesStore.entries.map((e) => e.name))
  return files.map((f) => f.name).filter((name) => existing.has(name))
}

function findFreeName(filename: string, taken: Set<string>): string {
  if (!taken.has(filename)) return filename
  const lastDot = filename.lastIndexOf('.')
  const base = lastDot >= 0 ? filename.slice(0, lastDot) : filename
  const ext = lastDot >= 0 ? filename.slice(lastDot) : ''
  let n = 1
  while (taken.has(`${base} (${n})${ext}`)) n++
  return `${base} (${n})${ext}`
}

function resolveKeepAll(files: File[]): File[] {
  const taken = new Set(filesStore.entries.map((e) => e.name))
  return files.map((file) => {
    const newName = findFreeName(file.name, taken)
    taken.add(newName)
    if (newName === file.name) return file
    return new File([file], newName, { type: file.type, lastModified: file.lastModified })
  })
}

function onUploadReplace() {
  uploadConflictDialog.value = false
  performUpload(pendingUploadFiles.value)
  pendingUploadFiles.value = []
}

function onUploadKeepAll() {
  uploadConflictDialog.value = false
  performUpload(resolveKeepAll(pendingUploadFiles.value))
  pendingUploadFiles.value = []
}

function openFilePicker() {
  uploadInput.value?.click()
}

async function handleFileInput(e: Event) {
  const files = Array.from((e.target as HTMLInputElement).files ?? [])
  if (!files.length) return
  await uploadFiles(files)
  if (uploadInput.value) uploadInput.value.value = ''
}

async function uploadFiles(files: File[]) {
  const conflicts = detectConflicts(files)
  if (conflicts.length > 0) {
    pendingUploadFiles.value = files
    conflictingNames.value = conflicts
    uploadConflictDialog.value = true
    return
  }
  await performUpload(files)
}

async function performUpload(files: File[]) {
  uploading.value = true
  try {
    const res = await fileops.upload(filesStore.currentPath, files)
    showToast(`Uploaded ${res.count} file${res.count !== 1 ? 's' : ''}`, 'success')
    await filesStore.refresh()
    // Reconnect the SSE stream so enqueueFolder rescans and picks up newly
    // uploaded files that weren't present when the stream first connected.
    thumbConnect(filesStore.currentPath)
  } catch (err) {
    showToast((err as Error).message, 'error')
  } finally {
    uploading.value = false
  }
}

// ── Drag & Drop ────────────────────────────────────────────────────────────
const dragCounter = ref(0)
const isDragging = computed(() => dragCounter.value > 0)

function onDragEnter(e: DragEvent) {
  if (e.dataTransfer?.types.includes('Files')) dragCounter.value++
}
function onDragLeave() {
  dragCounter.value = Math.max(0, dragCounter.value - 1)
}
function onDragOver(e: DragEvent) {
  if (e.dataTransfer?.types.includes('Files')) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
  }
}
async function onDrop(e: DragEvent) {
  e.preventDefault()
  dragCounter.value = 0
  const files = Array.from(e.dataTransfer?.files ?? [])
  if (files.length) await uploadFiles(files)
}

// ── New folder ────────────────────────────────────────────────────────────
const newFolderDialog = ref(false)
const newFolderName = ref('')
const newFolderInput = ref<HTMLInputElement | null>(null)

async function openNewFolder() {
  newFolderName.value = ''
  newFolderDialog.value = true
  await nextTick()
  newFolderInput.value?.focus()
}

async function confirmNewFolder() {
  const name = newFolderName.value.trim()
  if (!name) return
  newFolderDialog.value = false
  try {
    const path = filesStore.currentPath ? `${filesStore.currentPath}/${name}` : name
    await fileops.mkdir(path)
    filesStore.refresh()
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

// ── Rename ────────────────────────────────────────────────────────────────
const renameDialog = ref(false)
const renameDialogEntry = ref<{ path: string; name: string } | null>(null)
const renameDialogName = ref('')
const renameDialogInput = ref<HTMLInputElement | null>(null)

async function startRename(entry: { path: string; name: string }) {
  renameDialogEntry.value = entry
  renameDialogName.value = entry.name
  renameDialog.value = true
  await nextTick()
  renameDialogInput.value?.select()
}

async function confirmRename() {
  const entry = renameDialogEntry.value
  if (!entry) return
  const trimmed = renameDialogName.value.trim()
  renameDialog.value = false
  if (!trimmed || trimmed === (entry.path.split('/').pop() ?? '')) return
  const wasPreviewingThis = previewIndex.value >= 0 && previewEntry.value?.path === entry.path
  try {
    await fileops.rename(entry.path, trimmed)
    await filesStore.refresh()
    if (wasPreviewingThis) {
      const parent = entry.path.includes('/') ? entry.path.slice(0, entry.path.lastIndexOf('/')) : ''
      const newPath = parent ? `${parent}/${trimmed}` : trimmed
      const newIdx = previewableEntries.value.findIndex((e) => e.path === newPath)
      previewIndex.value = newIdx >= 0 ? newIdx : -1
    }
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

function cancelRename() {
  renameDialog.value = false
}

// ── Context menu ──────────────────────────────────────────────────────────
const contextMenuState = ref<{ x: number; y: number; entry: FileEntry; paths: string[] } | null>(null)

function onContextMenu(entry: FileEntry, x: number, y: number) {
  const paths = selectedPaths.value.includes(entry.path)
    ? [...selectedPaths.value]
    : [entry.path]
  contextMenuState.value = { x, y, entry, paths }
}

// ── Details panel ─────────────────────────────────────────────────────────
type SizeProgress = { files: number; dirs: number; bytes: number; done: boolean }
type MediaMeta = {
  image?: { width: number; height: number }
  video?: { duration: number; width: number; height: number; codec?: string }
}

const detailsItems = ref<FileEntry[]>([])
const sizeProgress = ref<SizeProgress | null>(null)
const detailsMedia = ref<MediaMeta | null>(null)
let sizeEventSource: EventSource | null = null

const detailsLocation = computed(() => {
  if (!detailsItems.value.length) return ''
  const parents = [...new Set(detailsItems.value.map((e) => {
    const slash = e.path.lastIndexOf('/')
    return slash < 0 ? '/' : '/' + e.path.slice(0, slash)
  }))]
  return parents.length === 1 ? parents[0] : 'Multiple locations'
})

const detailsFileCount = computed(() => detailsItems.value.filter((e) => !e.is_dir).length)
const detailsFolderCount = computed(() => detailsItems.value.filter((e) => e.is_dir).length)

const detailsSizeText = computed(() => {
  if (!sizeProgress.value) return null
  const { files, dirs, bytes, done } = sizeProgress.value
  const sizeStr = bytes > 0 ? formatSize(bytes) : (done ? '0 B' : null)
  const parts: string[] = []
  if (files > 0) parts.push(`${files.toLocaleString()} file${files !== 1 ? 's' : ''}`)
  if (dirs > 0) parts.push(`${dirs.toLocaleString()} folder${dirs !== 1 ? 's' : ''}`)
  if (done && parts.length === 0) return 'Empty'
  const countStr = parts.length > 0
    ? `(${!done ? 'scanning… ' : ''}${parts.join(', ')})`
    : '(scanning…)'
  if (sizeStr) return `${sizeStr} ${countStr}`.trim()
  return countStr
})

function formatDuration(secs: number): string {
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  const s = Math.floor(secs % 60)
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}

function entriesForPaths(paths: string[]): FileEntry[] {
  const pool = [...visibleEntries.value, ...searchResults.value]
  return paths.map((p) => pool.find((e) => e.path === p)).filter((e): e is FileEntry => !!e)
}

function openDetails(items: FileEntry[]) {
  sizeEventSource?.close()
  sizeEventSource = null
  detailsItems.value = items
  sizeProgress.value = null
  detailsMedia.value = null

  const needsStream = items.length > 1 || (items.length === 1 && items[0].is_dir)
  if (needsStream) {
    sizeProgress.value = { files: 0, dirs: 0, bytes: 0, done: false }
    const url = filesApi.sizeStreamUrl(items.map((e) => e.path))
    sizeEventSource = new EventSource(url)
    sizeEventSource.onmessage = (ev) => {
      sizeProgress.value = JSON.parse(ev.data)
      if (sizeProgress.value?.done) {
        sizeEventSource?.close()
        sizeEventSource = null
      }
    }
    sizeEventSource.onerror = () => {
      sizeEventSource?.close()
      sizeEventSource = null
    }
  }

  if (items.length === 1 && !items[0].is_dir) {
    const cat = fileCategory(items[0].mime, items[0].name)
    if (cat === 'image' || cat === 'video') {
      filesApi.fileMeta(items[0].path).then((meta) => {
        detailsMedia.value = meta
      }).catch(() => {})
    }
  }
}

function closeDetails() {
  detailsItems.value = []
  sizeEventSource?.close()
  sizeEventSource = null
  sizeProgress.value = null
  detailsMedia.value = null
}

onUnmounted(() => { sizeEventSource?.close() })

const contextMenuItems = computed((): ContextMenuItem[] => {
  if (!contextMenuState.value) return []
  const { entry, paths } = contextMenuState.value
  const single = paths.length === 1
  const close = () => { contextMenuState.value = null }
  const items: ContextMenuItem[] = [
    {
      label: 'Copy',
      icon: 'M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z',
      action: () => { clipboard.value = { mode: 'copy', paths }; close() },
    },
    {
      label: 'Move',
      icon: 'M5 12h14m-7-7l7 7-7 7',
      action: () => {
        clipboard.value = { mode: 'move', paths }
        selectedPaths.value = selectedPaths.value.filter((p) => !paths.includes(p))
        close()
      },
    },
    {
      label: 'Download',
      icon: 'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4',
      action: () => { close(); downloadPaths(paths) },
    },
    { separator: true },
  ]
  if (single) {
    items.push({
      label: 'Rename',
      icon: 'M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z',
      action: () => { close(); startRename(entry) },
    })
  }
  items.push({
    label: 'Delete',
    icon: 'M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16',
    danger: true,
    action: () => { close(); openDeleteDialog(paths, single ? entry.name : undefined) },
  })
  items.push({ separator: true })
  items.push({
    label: 'Details',
    icon: 'M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    action: () => { close(); openDetails(entriesForPaths(paths)) },
  })
  return items
})

// ── Toast ──────────────────────────────────────────────────────────────────
type ToastKind = 'success' | 'error'
const toast = ref<{ message: string; kind: ToastKind } | null>(null)
let toastTimer: ReturnType<typeof setTimeout> | null = null

function showToast(message: string, kind: ToastKind) {
  if (toastTimer) clearTimeout(toastTimer)
  toast.value = { message, kind }
  toastTimer = setTimeout(() => { toast.value = null }, 3000)
}

// ── Computed helpers ──────────────────────────────────────────────────────
const fileopsActive = computed(() => selectedPaths.value.length > 0 || clipboard.value !== null)

onMounted(() => {
  const initialPath = (route.query.path as string) || ''
  thumbConnect(initialPath)
  filesStore.navigate(initialPath)
  fetchFolderPref(initialPath)
  if (route.query.q) searchQuery.value = route.query.q as string
})
</script>

<template>
  <div
    class="h-full flex flex-col"
    @dragenter="onDragEnter"
    @dragleave="onDragLeave"
    @dragover="onDragOver"
    @drop="onDrop"
  >
    <!-- Toolbar: fixed 2-row height — no layout shift when fileops appear -->
    <div class="border-b border-zinc-800 bg-zinc-900 shrink-0">

      <!-- Row 1: Breadcrumbs -->
      <div class="px-3 h-9 flex items-center border-b border-zinc-800/50">
        <FileBreadcrumb :path="filesStore.currentPath" @navigate="navigate" />
      </div>

      <!-- Row 2: Search/FileOps (left, flex-1) + Action buttons (right, fixed) -->
      <div class="px-3 h-11 flex items-center gap-2">

        <!-- Left: swaps between search and fileops — same height, no layout shift -->
        <div class="flex-1 min-w-0 flex items-center gap-1.5 overflow-hidden">

          <!-- ── FileOps mode ── -->
          <template v-if="fileopsActive">
            <!-- Selection actions -->
            <template v-if="selectedPaths.length > 0">
              <!-- Deselect + count -->
              <button
                class="flex items-center gap-1.5 text-sm text-zinc-200 hover:text-white shrink-0 pl-0.5 pr-2 py-1 rounded hover:bg-zinc-800 transition-colors"
                title="Clear selection"
                @click="deselectAll"
              >
                <svg class="size-4 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
                {{ selectedPaths.length }} selected
              </button>

              <div class="w-px h-4 bg-zinc-700 mx-0.5 shrink-0" />

              <button
                class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0"
                title="Copy"
                @click="onCopy"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
              </button>

              <button
                class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0"
                title="Move"
                @click="onMove"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14m-7-7l7 7-7 7" />
                </svg>
              </button>

              <button
                :disabled="downloading"
                class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 disabled:opacity-40 transition-colors shrink-0"
                :title="downloading ? 'Zipping…' : 'Download'"
                @click="onDownload"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                </svg>
              </button>

              <button
                v-if="selectedPaths.length === 1"
                class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0"
                title="Rename"
                @click="startRename([...visibleEntries, ...searchResults].find(e => e.path === selectedPaths[0])!)"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                </svg>
              </button>

              <button
                class="p-1.5 rounded text-zinc-400 hover:text-red-400 hover:bg-zinc-700 transition-colors shrink-0"
                title="Delete"
                @click="openDeleteDialog([...selectedPaths])"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
              </button>

              <button
                class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0"
                title="Details"
                @click="openDetails(entriesForPaths([...selectedPaths]))"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </button>
            </template>

            <!-- Separator between selection actions and clipboard -->
            <div
              v-if="selectedPaths.length > 0 && clipboard"
              class="w-px h-4 bg-zinc-700 mx-0.5 shrink-0"
            />

            <!-- Clipboard -->
            <template v-if="clipboard">
              <span class="text-xs text-zinc-400 italic shrink-0 truncate">
                {{ clipboard.mode === 'copy' ? 'Copying' : 'Moving' }}
                {{ clipboard.paths.length }} item{{ clipboard.paths.length !== 1 ? 's' : '' }}
              </span>
              <button
                class="p-1.5 rounded text-blue-400 hover:text-blue-300 hover:bg-zinc-700 transition-colors shrink-0"
                title="Paste here"
                @click="onPaste"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
                </svg>
              </button>
              <button
                class="p-1 rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors shrink-0"
                title="Clear clipboard"
                @click="clipboard = null"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </template>
          </template>

          <!-- ── Search mode ── -->
          <div v-else class="relative w-full">
            <svg
              class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none"
              fill="none" viewBox="0 0 24 24" stroke="currentColor"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M21 21l-4.35-4.35M17 11A6 6 0 115 11a6 6 0 0112 0z" />
            </svg>
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Search files…"
              class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 text-sm rounded-lg pl-8 pr-7 py-1.5 border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
              @keydown.escape="clearSearch"
            />
            <button
              v-if="searchQuery"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300 transition-colors"
              @click="clearSearch"
            >
              <svg class="size-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Right: action buttons (always visible) -->
        <div class="flex items-center gap-0.5 shrink-0">
          <!-- New folder -->
          <button
            class="p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800"
            title="New folder"
            @click="openNewFolder"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M9 13h6m-3-3v6m-9 1V7a2 2 0 012-2h6l2 2h6a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2z" />
            </svg>
          </button>

          <!-- Upload -->
          <button
            :class="['p-1.5 rounded transition-colors', uploading ? 'text-blue-400 animate-pulse' : 'text-zinc-400 hover:text-white hover:bg-zinc-800']"
            title="Upload files"
            @click="openFilePicker"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
            </svg>
          </button>
          <input ref="uploadInput" type="file" multiple class="hidden" @change="handleFileInput" />

          <!-- View toggle -->
          <div class="flex items-center bg-zinc-800 rounded-lg p-0.5 gap-0.5 ml-0.5">
            <button
              :class="['p-1.5 rounded-md transition-colors', viewMode === 'grid' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white']"
              title="Grid view"
              @click="setView('grid')"
            >
              <svg class="size-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M4 4h6v6H4V4zm0 10h6v6H4v-6zm10-10h6v6h-6V4zm0 10h6v6h-6v-6z" />
              </svg>
            </button>
            <button
              :class="['p-1.5 rounded-md transition-colors', viewMode === 'list' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white']"
              title="List view"
              @click="setView('list')"
            >
              <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M4 6h16M4 10h16M4 14h16M4 18h16" />
              </svg>
            </button>
          </div>

          <!-- Show/hide hidden files -->
          <button
            :class="['p-1.5 rounded transition-colors hover:bg-zinc-800', showHidden ? 'text-blue-400 hover:text-blue-300' : 'text-zinc-400 hover:text-white']"
            :title="showHidden ? 'Hide hidden files' : 'Show hidden files'"
            @click="toggleHidden"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
            </svg>
          </button>

          <!-- Refresh -->
          <button
            class="p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800"
            title="Refresh"
            @click="filesStore.refresh()"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <!-- Error banner -->
    <div
      v-if="filesStore.error"
      class="px-4 py-2 text-sm text-red-400 bg-red-950/30 border-b border-red-900 shrink-0"
    >
      {{ filesStore.error }}
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-hidden relative">
      <!-- Drag & drop overlay -->
      <Transition name="fade">
        <div
          v-if="isDragging"
          class="absolute inset-0 z-20 bg-blue-950/60 border-2 border-dashed border-blue-400 flex flex-col items-center justify-center gap-3 pointer-events-none"
        >
          <svg class="size-12 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
              d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
          </svg>
          <span class="text-blue-200 text-lg font-medium">Drop files to upload</span>
        </div>
      </Transition>

      <FileSearch
        v-if="searchQuery"
        :entries="searchResults"
        :loading="searchLoading"
        :query="searchQuery"
        @navigate="(p) => { navigate(p); clearSearch() }"
        @preview="openPreview"
        @archive-preview="archiveEntry = $event"
      />
      <FileGrid
        v-else-if="viewMode === 'grid'"
        :entries="visibleEntries"
        :loading="filesStore.loading || prefLoading"
        :selected="selectedPaths"

        @navigate="navigate"
        @preview="openPreview"
        @archive-preview="archiveEntry = $event"
        @toggle-select="toggleSelect"
        @context-menu="onContextMenu"
      />
      <FileList
        v-else
        :entries="visibleEntries"
        :loading="filesStore.loading || prefLoading"
        :current-path="filesStore.currentPath"
        :sort-key="sortKey"
        :sort-desc="sortDesc"
        :selected="selectedPaths"
        @navigate="navigate"
        @preview="openPreview"
        @archive-preview="archiveEntry = $event"
        @toggle-sort="toggleSort"
        @toggle-select="toggleSelect"
        @select-all="selectAll"
        @deselect-all="deselectAll"
        @context-menu="onContextMenu"
      />
    </div>

    <FilePreview
      :entry="previewEntry"
      :has-prev="previewIndex > 0"
      :has-next="previewIndex < previewableEntries.length - 1"
      :index="previewIndex"
      :total="previewableEntries.length"
      @close="closePreview"
      @prev="prevPreview"
      @next="nextPreview"
      @rename="startRename"
      @delete="(e) => openDeleteDialog([e.path], e.name)"
    />

    <ArchiveContents
      v-if="archiveEntry"
      :entry="archiveEntry"
      @close="archiveEntry = null"
      @extracted="onExtracted"
    />

    <!-- New folder dialog -->
    <Transition name="fade">
      <div
        v-if="newFolderDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        @click.self="newFolderDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">New folder</h3>
          <input
            ref="newFolderInput"
            v-model="newFolderName"
            type="text"
            placeholder="Folder name"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
            @keydown.enter="confirmNewFolder"
            @keydown.escape="newFolderDialog = false"
          />
          <div class="flex gap-2 mt-3 justify-end">
            <button
              class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
              @click="newFolderDialog = false"
            >Cancel</button>
            <button
              class="px-3 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
              @click="confirmNewFolder"
            >Create</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Rename dialog -->
    <Transition name="fade">
      <div
        v-if="renameDialog"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
        @click.self="cancelRename"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">Rename</h3>
          <input
            ref="renameDialogInput"
            v-model="renameDialogName"
            type="text"
            placeholder="New name"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
            @keydown.enter="confirmRename"
            @keydown.escape="cancelRename"
          />
          <div class="flex gap-2 mt-3 justify-end">
            <button
              class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
              @click="cancelRename"
            >Cancel</button>
            <button
              class="px-3 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
              @click="confirmRename"
            >Rename</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Delete confirmation dialog -->
    <Transition name="fade">
      <div
        v-if="deleteDialog"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
        @click.self="cancelDelete"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-1">
            {{ deletePending && deletePending.paths.length === 1 ? 'Delete file?' : `Delete ${deletePending?.paths.length} files?` }}
          </h3>
          <p class="text-xs text-zinc-400 mb-3">
            <template v-if="deletePending && deletePending.paths.length === 1">
              This will delete <span class="text-zinc-200">{{ deletePending.name ?? 'this file' }}</span> from your files.
            </template>
            <template v-else>
              This will delete {{ deletePending?.paths.length }} items from your files.
            </template>
          </p>
          <div class="flex gap-2 mt-3 justify-end">
            <button
              class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
              @click="cancelDelete"
            >Cancel</button>
            <button
              class="px-3 py-1.5 text-sm bg-red-700 hover:bg-red-600 text-white rounded-lg transition-colors"
              @click="confirmDelete"
            >Delete</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Upload conflict dialog -->
    <Transition name="fade">
      <div
        v-if="uploadConflictDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        @click.self="uploadConflictDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-80 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-1">Upload conflicts</h3>
          <p class="text-xs text-zinc-400 mb-3">
            {{ conflictingNames.length }} file{{ conflictingNames.length !== 1 ? 's' : '' }} already exist in this folder.
          </p>
          <ul class="mb-4 space-y-1 max-h-36 overflow-y-auto">
            <li
              v-for="name in conflictingNames"
              :key="name"
              class="text-xs text-zinc-300 truncate bg-zinc-800 rounded px-2 py-1"
            >{{ name }}</li>
          </ul>
          <div class="flex gap-2 justify-end">
            <button
              class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
              @click="uploadConflictDialog = false"
            >Cancel</button>
            <button
              class="px-3 py-1.5 text-sm bg-zinc-700 hover:bg-zinc-600 text-white rounded-lg transition-colors"
              @click="onUploadReplace"
            >Replace</button>
            <button
              class="px-3 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
              @click="onUploadKeepAll"
            >Keep all</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Context menu -->
    <ContextMenu
      v-if="contextMenuState"
      :x="contextMenuState.x"
      :y="contextMenuState.y"
      :items="contextMenuItems"
      @close="contextMenuState = null"
    />

    <!-- Details modal -->
    <Transition name="fade">
      <div
        v-if="detailsItems.length > 0"
        class="fixed inset-0 z-[70] flex items-center justify-center bg-black/50"
        @click.self="closeDetails"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-96 shadow-2xl">
          <!-- Header -->
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">
            <template v-if="detailsItems.length === 1">Details</template>
            <template v-else>
              Details —
              <span v-if="detailsFolderCount > 0">{{ detailsFolderCount.toLocaleString() }} folder{{ detailsFolderCount !== 1 ? 's' : '' }}</span>
              <span v-if="detailsFolderCount > 0 && detailsFileCount > 0">, </span>
              <span v-if="detailsFileCount > 0">{{ detailsFileCount.toLocaleString() }} file{{ detailsFileCount !== 1 ? 's' : '' }}</span>
            </template>
          </h3>

          <dl class="space-y-2">
            <!-- Name (single only) -->
            <div v-if="detailsItems.length === 1" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Name</dt>
              <dd class="text-xs text-zinc-200 break-all">{{ detailsItems[0].name }}</dd>
            </div>

            <!-- Location -->
            <div class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Location</dt>
              <dd class="text-xs text-zinc-400 break-all font-mono">{{ detailsLocation }}</dd>
            </div>

            <!-- Type (single only) -->
            <div v-if="detailsItems.length === 1" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Type</dt>
              <dd class="text-xs text-zinc-200">{{ detailsItems[0].is_dir ? 'Folder' : (detailsItems[0].mime || 'Unknown') }}</dd>
            </div>

            <!-- Size -->
            <div class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Size</dt>
              <dd class="text-xs text-zinc-200">
                <!-- Single file: show directly -->
                <template v-if="detailsItems.length === 1 && !detailsItems[0].is_dir">
                  {{ formatSize(detailsItems[0].size) }}
                </template>
                <!-- Streaming (folder or multi) -->
                <template v-else>{{ detailsSizeText }}</template>
              </dd>
            </div>

            <!-- Created (single only, if filesystem provides it) -->
            <div v-if="detailsItems.length === 1 && detailsItems[0].created_at" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Created</dt>
              <dd class="text-xs text-zinc-200">{{ formatDate(detailsItems[0].created_at) }}</dd>
            </div>

            <!-- Modified (single only) -->
            <div v-if="detailsItems.length === 1" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Modified</dt>
              <dd class="text-xs text-zinc-200">{{ formatDate(detailsItems[0].mod_time) }}</dd>
            </div>

            <!-- Image resolution -->
            <div v-if="detailsMedia?.image" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Resolution</dt>
              <dd class="text-xs text-zinc-200">{{ detailsMedia.image.width }} × {{ detailsMedia.image.height }}</dd>
            </div>

            <!-- Video metadata -->
            <template v-if="detailsMedia?.video">
              <div class="flex gap-2">
                <dt class="text-xs text-zinc-500 w-20 shrink-0">Duration</dt>
                <dd class="text-xs text-zinc-200">{{ formatDuration(detailsMedia.video.duration) }}</dd>
              </div>
              <div class="flex gap-2">
                <dt class="text-xs text-zinc-500 w-20 shrink-0">Resolution</dt>
                <dd class="text-xs text-zinc-200">{{ detailsMedia.video.width }} × {{ detailsMedia.video.height }}</dd>
              </div>
              <div v-if="detailsMedia.video.codec" class="flex gap-2">
                <dt class="text-xs text-zinc-500 w-20 shrink-0">Codec</dt>
                <dd class="text-xs text-zinc-200">{{ detailsMedia.video.codec }}</dd>
              </div>
            </template>
          </dl>

          <div class="flex justify-end mt-4">
            <button
              class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
              @click="closeDetails"
            >Close</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Toast -->
    <Transition name="toast">
      <div
        v-if="toast"
        :class="[
          'fixed bottom-6 left-1/2 -translate-x-1/2 z-50 px-4 py-2.5 rounded-lg shadow-xl text-sm font-medium pointer-events-none',
          toast.kind === 'success' ? 'bg-green-800 text-green-100' : 'bg-red-800 text-red-100',
        ]"
      >
        {{ toast.message }}
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(8px);
}
</style>
