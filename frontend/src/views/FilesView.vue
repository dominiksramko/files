<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useFilesStore } from '@/stores/files'
import type { FileEntry, FileOp } from '@/types'
import { fileCategory, formatSize, formatDate } from '@/utils/formatters'
import { files as filesApi, folders as foldersApi, fileops, ConflictError } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import FileList from '@/components/files/FileList.vue'
import FileGrid from '@/components/files/FileGrid.vue'
import FileSearch from '@/components/files/FileSearch.vue'
import FilePreview from '@/components/files/FilePreview.vue'
import FileOpsBar from '@/components/files/FileOpsBar.vue'
import ArchiveContents from '@/components/archive/ArchiveContents.vue'
import ContextMenu from '@/components/ui/ContextMenu.vue'
import type { ContextMenuItem } from '@/components/ui/ContextMenu.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import ConflictDialog from '@/components/ui/ConflictDialog.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { useConflictDialog } from '@/composables/useConflictDialog'

const filesStore = useFilesStore()
const route = useRoute()
const router = useRouter()
const { prompt: promptConflict } = useConflictDialog()

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
const searchExpanded = ref(false) // mobile: search bar expanded
const mobileSearchOpen = computed(() => searchExpanded.value || searchQuery.value.trim().length > 0)
const mobileSearchInputRef = ref<HTMLInputElement | null>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSource: EventSource | null = null

function closeSearchSource() {
  if (searchSource) {
    searchSource.close()
    searchSource = null
  }
}

function openMobileSearch() {
  searchExpanded.value = true
  nextTick(() => mobileSearchInputRef.value?.focus())
}

function startSearch(query: string) {
  if (searchTimer) clearTimeout(searchTimer)
  closeSearchSource()
  searchResults.value = []
  searchLoading.value = true
  searchTimer = setTimeout(() => {
    const url = filesApi.searchStreamUrl(query.trim(), '', showHidden.value)
    const es = new EventSource(url)
    searchSource = es
    es.onmessage = (e) => {
      try {
        const obj = JSON.parse(e.data as string)
        if (obj.done) {
          searchLoading.value = false
          es.close()
          if (searchSource === es) searchSource = null
          return
        }
        searchResults.value.push(obj as FileEntry)
      } catch { /* ignore malformed */ }
    }
    es.onerror = () => {
      searchLoading.value = false
      es.close()
      if (searchSource === es) searchSource = null
    }
  }, 300)
}

watch(searchQuery, (query) => {
  if (((route.query.q as string) ?? '') !== query) {
    router.replace({ query: routeQuery() })
  }
  if (!query.trim()) {
    if (searchTimer) clearTimeout(searchTimer)
    closeSearchSource()
    searchResults.value = []
    searchLoading.value = false
    return
  }
  startSearch(query)
})

watch(showHidden, () => {
  if (searchQuery.value.trim()) startSearch(searchQuery.value)
})

function clearSearch() {
  closeSearchSource()
  if (searchTimer) clearTimeout(searchTimer)
  searchExpanded.value = false
  searchQuery.value = ''
  searchResults.value = []
  searchLoading.value = false
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
const previewFilePath    = ref<string | null>(null)
const previewTextEditMode = ref(false)
const archiveEntry       = ref<FileEntry | null>(null)

const previewableEntries = computed(() => {
  const pool = searchQuery.value ? searchResults.value : visibleEntries.value
  return pool.filter((e) => !e.is_dir && fileCategory(e.mime, e.name) !== 'archive')
})

// Derived from previewFilePath so it auto-corrects whenever the list changes
// (e.g. a new file added by SSE shifts positions).
const previewIndex = computed(() =>
  previewFilePath.value !== null
    ? previewableEntries.value.findIndex((entry) => entry.path === previewFilePath.value)
    : -1
)

const previewEntry = computed<FileEntry | null>(
  () => previewableEntries.value[previewIndex.value] ?? null,
)

function openPreview(entry: FileEntry) {
  previewTextEditMode.value = false
  previewFilePath.value = entry.path
}
function closePreview() { previewFilePath.value = null }

function prevPreview() {
  const previousEntry = previewableEntries.value[previewIndex.value - 1]
  if (previousEntry) { previewTextEditMode.value = false; previewFilePath.value = previousEntry.path }
}
function nextPreview() {
  const nextEntry = previewableEntries.value[previewIndex.value + 1]
  if (nextEntry) { previewTextEditMode.value = false; previewFilePath.value = nextEntry.path }
}

// ── URL-synced navigation ──────────────────────────────────────────────────
function routeQuery() {
  const q: Record<string, string> = {}
  if (filesStore.currentPath) q.path = filesStore.currentPath
  if (searchQuery.value) q.q = searchQuery.value
  if (previewEntry.value) q.preview = previewEntry.value.path
  return q
}

async function navigate(path: string) {
  const actualPath = await filesStore.navigate(path)
  fetchFolderPref(actualPath)
  selectedPaths.value = []
  lastAnchorPath.value = null
  router.push({ query: actualPath ? { path: actualPath } : {} })
}

watch(previewEntry, (entry) => {
  const currentPreview = route.query.preview as string | undefined
  if (entry?.path === currentPreview) return
  router.replace({ query: routeQuery() })
})

watch(
  () => route.query,
  async (q) => {
    const p = (q.path as string) || ''
    if (p !== filesStore.currentPath) {
      const actualPath = await filesStore.navigate(p)
      fetchFolderPref(actualPath)
      selectedPaths.value = []
      lastAnchorPath.value = null
      if (actualPath !== p) {
        router.replace({ query: actualPath ? { path: actualPath } : {} })
        return
      }
    }
    const qParam = (q.q as string) ?? ''
    if (searchQuery.value !== qParam) searchQuery.value = qParam

    const urlPreviewPath = q.preview as string | undefined
    if (previewFilePath.value === (urlPreviewPath ?? null)) return
    previewFilePath.value = urlPreviewPath ?? null
  },
)

// ── Archive ────────────────────────────────────────────────────────────────
function onExtracted() {
  archiveEntry.value = null
}

// ── Selection ──────────────────────────────────────────────────────────────
const selectedPaths = ref<string[]>([])
const lastAnchorPath = ref<string | null>(null)

function toggleSelect(path: string, shift: boolean) {
  const list = visibleEntries.value.map((e) => e.path)
  if (shift && lastAnchorPath.value && list.includes(lastAnchorPath.value)) {
    const a = list.indexOf(lastAnchorPath.value)
    const b = list.indexOf(path)
    const [lo, hi] = a < b ? [a, b] : [b, a]
    const range = list.slice(lo, hi + 1)
    const alreadySelected = range.every((p) => selectedPaths.value.includes(p))
    if (alreadySelected) {
      selectedPaths.value = selectedPaths.value.filter((p) => !range.includes(p))
    } else {
      const merged = new Set([...selectedPaths.value, ...range])
      selectedPaths.value = list.filter((p) => merged.has(p))
    }
    return
  }
  lastAnchorPath.value = path
  const idx = selectedPaths.value.indexOf(path)
  if (idx >= 0) selectedPaths.value = selectedPaths.value.filter((_, i) => i !== idx)
  else selectedPaths.value = [...selectedPaths.value, path]
}

function selectAll() {
  selectedPaths.value = visibleEntries.value.map((e) => e.path)
}

function deselectAll() {
  selectedPaths.value = []
  lastAnchorPath.value = null
}

// ── Clipboard ──────────────────────────────────────────────────────────────
const clipboard = ref<{ mode: 'copy' | 'move'; paths: string[] } | null>(null)

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
      action: () => { after(); downloadPaths(paths) },
    },
    {
      key: 'compress', label: 'Compress', icon: 'archive',
      action: () => { after(); openCompressDialog(paths) },
    },
    {
      key: 'extract', label: 'Extract here', icon: 'archiveOut',
      visible: single && !!entry?.can_extract,
      action: () => { after(); openExtract(entry!.path) },
    },
    {
      key: 'rename', label: 'Rename', icon: 'pencil',
      visible: single,
      action: () => { after(); if (entry) startRename(entry) },
    },
    {
      key: 'delete', label: 'Delete', icon: 'trash', danger: true,
      action: () => { after(); openDeleteDialog(paths) },
    },
    {
      key: 'details', label: 'Details', icon: 'info',
      action: () => { after(); openDetails(entriesForPaths(paths)) },
    },
  ]
}

const fileOps = computed((): FileOp[] => {
  if (selectedPaths.value.length === 0) return []
  const paths = [...selectedPaths.value]
  const pool = [...visibleEntries.value, ...searchResults.value]
  const entry = selectedPaths.value.length === 1 ? (pool.find(e => e.path === selectedPaths.value[0]) ?? null) : null
  return buildFileOps(paths, entry)
})

async function onPaste() {
  if (!clipboard.value) return
  const { mode, paths } = clipboard.value
  await doPaste(mode, paths, filesStore.currentPath)
}

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

// ── Background jobs (upload progress, zip download) ────────────────────────
interface UploadJob   { kind: 'upload';   id: number; count: number; pct: number; loaded: number; total: number; startedAt: number; abort: () => void }
interface ZipJob      { kind: 'zip';      id: number }
interface CompressJob { kind: 'compress'; id: number; name: string }
type BgJob = UploadJob | ZipJob | CompressJob

const bgJobs = ref<BgJob[]>([])
let nextJobId = 0

const downloading = computed(() => bgJobs.value.some(j => j.kind === 'zip'))

// ── Selection-clear tracking for background jobs ────────────────────────────
// Only the last-started job may clear selection, and only if the user hasn't
// manually changed it since that job started.
let lastBgJobId = -1
let selectionChangedSinceLastJob = false
let silentSelectionChange = false

watch(selectedPaths, () => {
  if (!silentSelectionChange) selectionChangedSinceLastJob = true
})

// When a path is removed via the watch stream, silently drop it from selection.
watch(() => filesStore.lastRemovedPath, (path) => {
  if (!path || !selectedPaths.value.includes(path)) return
  silentSelectionChange = true
  selectedPaths.value = selectedPaths.value.filter((p) => p !== path)
  silentSelectionChange = false
})

function startBgJobTracking(id: number) {
  lastBgJobId = id
  selectionChangedSinceLastJob = false
}

function canClearSelection(id: number): boolean {
  return id === lastBgJobId && !selectionChangedSinceLastJob
}

// ── Download selection ─────────────────────────────────────────────────────
async function downloadPaths(paths: string[]) {
  if (paths.length === 1) {
    const pool = [...visibleEntries.value, ...searchResults.value]
    const entry = pool.find((e) => e.path === paths[0])
    if (entry && !entry.is_dir) {
      const a = document.createElement('a')
      a.href = filesApi.downloadUrl(entry.path, entry.mod_time)
      a.click()
      selectedPaths.value = []
      return
    }
  }
  const id = nextJobId++
  startBgJobTracking(id)
  bgJobs.value.push({ kind: 'zip', id })
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
    if (canClearSelection(id)) selectedPaths.value = []
  } catch (err) {
    showToast((err as Error).message, 'error')
  } finally {
    bgJobs.value = bgJobs.value.filter(j => j.id !== id)
  }
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
    selectedPaths.value = []
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

function formatEta(job: UploadJob): string {
  if (job.loaded === 0 || job.total === 0) return ''
  const elapsed = (Date.now() - job.startedAt) / 1000
  if (elapsed < 2) return ''
  const speed = job.loaded / elapsed // bytes/sec
  const secs = (job.total - job.loaded) / speed
  if (secs < 5) return 'almost done'
  if (secs < 60) return `${Math.round(secs)}s remaining`
  const m = Math.floor(secs / 60), s = Math.round(secs % 60)
  return `${m}m ${s}s remaining`
}

// ── Upload conflict modal ──────────────────────────────────────────────────
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
    const choice = await promptConflict('Upload conflicts', conflicts)
    if (!choice) return
    files = choice === 'rename' ? resolveKeepAll(files) : files
  }
  await performUpload(files)
}

async function performUpload(files: File[]) {
  const totalSize = files.reduce((s, f) => s + f.size, 0)
  const id = nextJobId++
  const { promise, abort } = fileops.upload(filesStore.currentPath, files, (loaded, total) => {
    const job = bgJobs.value.find(j => j.id === id) as UploadJob | undefined
    if (job) { job.pct = Math.round((loaded / total) * 100); job.loaded = loaded; job.total = total }
  })
  bgJobs.value.push({ kind: 'upload', id, count: files.length, pct: 0, loaded: 0, total: totalSize, startedAt: Date.now(), abort })
  try {
    const res = await promise
    showToast(`Uploaded ${res.count} file${res.count !== 1 ? 's' : ''}`, 'success')
  } catch (err) {
    if ((err as Error).message !== 'cancelled') showToast((err as Error).message, 'error')
  } finally {
    bgJobs.value = bgJobs.value.filter(j => j.id !== id)
  }
}

// ── Upload Drag & Drop (OS files only — internal moves handled separately) ──
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
  if (!e.dataTransfer?.types.includes('Files')) return
  const files = Array.from(e.dataTransfer?.files ?? [])
  if (files.length) await uploadFiles(files)
}

// ── Internal drag-and-drop move ────────────────────────────────────────────
async function onDropToFolder(destPath: string, draggedPaths: string[]) {
  if (destPath === filesStore.currentPath) return
  for (const p of draggedPaths) {
    if (destPath === p || destPath.startsWith(p + '/')) return
  }
  await performMove(draggedPaths, destPath)
}

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
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

// ── New text file ─────────────────────────────────────────────────────────
const newTextFileDialog = ref(false)
const newTextFileName = ref('')
const newTextFileInput = ref<HTMLInputElement | null>(null)

async function openNewTextFile() {
  newTextFileName.value = ''
  newTextFileDialog.value = true
  await nextTick()
  newTextFileInput.value?.focus()
}

async function confirmNewTextFile() {
  const name = newTextFileName.value.trim()
  if (!name) return
  newTextFileDialog.value = false
  const filename = name.endsWith('.txt') ? name : `${name}.txt`
  const path = filesStore.currentPath ? `${filesStore.currentPath}/${filename}` : filename
  try {
    await filesApi.writeContent(path, '')
    previewTextEditMode.value = true
    previewFilePath.value = path
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
  const wasPreviewingThis = previewFilePath.value === entry.path
  try {
    await fileops.rename(entry.path, trimmed)
    selectedPaths.value = []
    // Optimistically update the entry so preview index can be resolved immediately.
    // The watch stream will deliver the authoritative REMOVE + ADD events shortly after.
    const parent = entry.path.includes('/') ? entry.path.slice(0, entry.path.lastIndexOf('/')) : ''
    const newPath = parent ? `${parent}/${trimmed}` : trimmed
    const idx = filesStore.entries.findIndex((e) => e.path === entry.path)
    if (idx >= 0) {
      const existing = filesStore.entries[idx]!
      filesStore.entries[idx] = { ...existing, name: trimmed, path: newPath }
    }
    if (wasPreviewingThis) {
      previewFilePath.value = newPath
    }
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

function cancelRename() {
  renameDialog.value = false
}

// ── Compress ──────────────────────────────────────────────────────────────
const compressDialog = ref(false)
const compressPaths = ref<string[]>([])
const compressName = ref('')
const compressFormat = ref('zip')
const compressPassword = ref('')
const compressProtectFilenames = ref(false)

const compressFormatOptions = [
  { label: 'ZIP (.zip)', value: 'zip' },
  { label: 'TAR (.tar)', value: 'tar' },
  { label: 'TAR.GZ (.tar.gz)', value: 'tar.gz' },
  { label: '7-Zip (.7z)', value: '7z' },
]

const compressExtMap: Record<string, string> = {
  zip: '.zip',
  tar: '.tar',
  'tar.gz': '.tar.gz',
  '7z': '.7z',
}

function openCompressDialog(paths: string[]) {
  compressPaths.value = paths
  const parts0 = paths.length === 1 ? (paths[0] as string).split('/').filter(Boolean) : []
  const singleName = parts0.length ? parts0[parts0.length - 1] : undefined
  const cwdParts = filesStore.currentPath.split('/').filter(Boolean)
  const baseName = singleName ?? (cwdParts.length ? cwdParts[cwdParts.length - 1] : 'archive')
  compressFormat.value = 'zip'
  compressName.value = baseName + '.zip'
  compressPassword.value = ''
  compressProtectFilenames.value = false
  compressDialog.value = true
}

watch(compressFormat, (fmt) => {
  const ext = compressExtMap[fmt] ?? '.zip'
  const stripped = compressName.value.replace(/\.(zip|tar\.gz|tar|7z)$/i, '')
  compressName.value = stripped + ext
})

function confirmCompress() {
  const name = compressName.value.trim()
  if (!name) return
  const dest = filesStore.currentPath ? filesStore.currentPath + '/' + name : name
  const pass = compressPassword.value || undefined
  const protect = compressFormat.value === '7z' && !!pass && compressProtectFilenames.value
  compressDialog.value = false
  runCompress([...compressPaths.value], dest, compressFormat.value, pass, protect)
}

async function runCompress(paths: string[], dest: string, format: string, password: string | undefined, protect: boolean, conflictMode: 'error' | 'replace' | 'rename' = 'error') {
  const name = dest.split('/').pop() ?? dest
  const id = nextJobId++
  startBgJobTracking(id)
  bgJobs.value.push({ kind: 'compress', id, name })
  try {
    await fileops.compress(paths, dest, format, password, protect, conflictMode)
    if (canClearSelection(id)) selectedPaths.value = []
    showToast('Archive created', 'success')
  } catch (err) {
    if (err instanceof ConflictError) {
      const choice = await promptConflict('Archive already exists', err.conflicts)
      if (choice) runCompress(paths, dest, format, password, protect, choice)
    } else {
      showToast(err instanceof Error ? err.message : 'Failed to create archive', 'error')
    }
  } finally {
    bgJobs.value = bgJobs.value.filter(j => j.id !== id)
  }
}

// ── Extract ───────────────────────────────────────────────────────────────
const extractPasswordDialog = ref(false)
const extractPasswordInput = ref('')
const extractPendingPath = ref('')
const extracting = ref(false)
const extractError = ref('')

async function openExtract(path: string) {
  await runExtract(path, undefined)
}

async function runExtract(path: string, password: string | undefined, conflictMode: 'error' | 'replace' | 'rename' = 'error') {
  try {
    await filesApi.extract(path, undefined, password, conflictMode)
    selectedPaths.value = []
    showToast('Extracted successfully', 'success')
  } catch (e) {
    if (e instanceof ConflictError) {
      const choice = await promptConflict('Extraction conflicts', e.conflicts)
      if (choice) runExtract(path, password, choice)
      return
    }
    const msg = e instanceof Error ? e.message : 'Extraction failed'
    if (msg.toLowerCase().includes('password') || msg.toLowerCase().includes('encrypted')) {
      extractPendingPath.value = path
      extractPasswordInput.value = ''
      extractError.value = ''
      extractPasswordDialog.value = true
    } else {
      showToast(msg, 'error')
    }
  }
}

async function confirmExtract() {
  if (!extractPasswordInput.value || extracting.value) return
  extracting.value = true
  extractError.value = ''
  try {
    await filesApi.extract(extractPendingPath.value, undefined, extractPasswordInput.value)
    extractPasswordDialog.value = false
    selectedPaths.value = []
    showToast('Extracted successfully', 'success')
  } catch (e) {
    if (e instanceof ConflictError) {
      extractPasswordDialog.value = false
      const choice = await promptConflict('Extraction conflicts', e.conflicts)
      if (choice) runExtract(extractPendingPath.value, extractPasswordInput.value, choice)
    } else {
      extractError.value = e instanceof Error ? e.message : 'Extraction failed'
    }
  } finally {
    extracting.value = false
  }
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

  const first = items[0]
  const needsStream = items.length > 1 || (items.length === 1 && first?.is_dir)
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

  if (first && items.length === 1 && !first.is_dir) {
    const cat = fileCategory(first.mime, first.name)
    if (cat === 'image' || cat === 'video') {
      filesApi.fileMeta(first.path).then((meta) => {
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
  items.push(toItem(get('delete')))
  items.push({ separator: true })
  items.push(toItem(get('details')))
  return items
})

// ── Toast ──────────────────────────────────────────────────────────────────
const { show: showToast } = useToast()

// ── Computed helpers ──────────────────────────────────────────────────────
const fileopsActive = computed(() => selectedPaths.value.length > 0 || clipboard.value !== null)

onMounted(async () => {
  const initialPath = (route.query.path as string) || ''
  const actualPath = await filesStore.navigate(initialPath)
  fetchFolderPref(actualPath)
  if (actualPath !== initialPath) {
    router.replace({ query: actualPath ? { path: actualPath } : {} })
  }
  if (route.query.preview) previewFilePath.value = route.query.preview as string
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
        <FileBreadcrumb :path="filesStore.currentPath" @navigate="navigate" @drop-to-folder="onDropToFolder" />
      </div>

      <!-- Row 2: Search/FileOps (left, flex-1) + Action buttons (right, fixed) -->
      <div class="px-3 h-11 flex items-center gap-2">

        <!-- MOBILE: full-width expanded search (replaces everything) -->
        <template v-if="mobileSearchOpen">
          <div class="relative flex-1 sm:hidden">
            <AppIcon name="search" class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none" />
            <input
              ref="mobileSearchInputRef"
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
              <AppIcon name="x" class="size-3.5" />
            </button>
          </div>
          <button
            class="sm:hidden shrink-0 p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors"
            title="Close search"
            @click="clearSearch"
          >
            <AppIcon name="x" class="size-4" />
          </button>
        </template>

        <!-- Left: swaps between search and fileops — hidden on mobile when search open -->
        <div :class="['flex-1 min-w-0 items-center gap-1.5 overflow-hidden', mobileSearchOpen ? 'hidden sm:flex' : 'flex']">

          <!-- ── FileOps mode ── -->
          <FileOpsBar
            v-if="fileopsActive"
            :ops="fileOps"
            :selected-count="selectedPaths.length"
            :clipboard="clipboard"
            @paste="onPaste"
            @clear-clipboard="clipboard = null"
            @deselect-all="deselectAll"
          />

          <!-- ── Search input — desktop only (sm+) ── -->
          <div v-else class="relative w-full hidden sm:block">
            <AppIcon name="search" class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none" />
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
              <AppIcon name="x" class="size-3.5" />
            </button>
          </div>
        </div>

        <!-- Right: action buttons — hidden on mobile when search open -->
        <div :class="['items-center gap-0.5 shrink-0', mobileSearchOpen ? 'hidden sm:flex' : 'flex']">

          <!-- Search icon — mobile only, not shown during fileops -->
          <button
            v-if="!fileopsActive"
            class="sm:hidden p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800"
            title="Search"
            @click="openMobileSearch"
          >
            <AppIcon name="search" class="size-4" />
          </button>

          <!-- New text file -->
          <button class="p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800" title="New text file" @click="openNewTextFile">
            <AppIcon name="newFile" class="size-4" />
          </button>

          <!-- New folder -->
          <button class="p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800" title="New folder" @click="openNewFolder">
            <AppIcon name="newFolder" class="size-4" />
          </button>

          <!-- Upload -->
          <button :class="['p-1.5 rounded transition-colors', bgJobs.some(j => j.kind === 'upload') ? 'text-blue-400 animate-pulse' : 'text-zinc-400 hover:text-white hover:bg-zinc-800']" title="Upload files" @click="openFilePicker">
            <AppIcon name="upload" class="size-4" />
          </button>
          <input ref="uploadInput" type="file" multiple class="hidden" @change="handleFileInput" />

          <!-- View toggle -->
          <div v-if="!searchQuery" class="flex items-center bg-zinc-800 rounded-lg p-0.5 gap-0.5 ml-0.5">
            <button :class="['p-1.5 rounded-md transition-colors', viewMode === 'grid' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white']" title="Grid view" @click="setView('grid')">
              <AppIcon name="grid" class="size-4" />
            </button>
            <button :class="['p-1.5 rounded-md transition-colors', viewMode === 'list' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white']" title="List view" @click="setView('list')">
              <AppIcon name="bars" class="size-4" />
            </button>
          </div>

          <!-- Show/hide hidden files -->
          <button :class="['p-1.5 rounded transition-colors hover:bg-zinc-800', showHidden ? 'text-blue-400 hover:text-blue-300' : 'text-zinc-400 hover:text-white']" :title="showHidden ? 'Hide hidden files' : 'Show hidden files'" @click="toggleHidden">
            <AppIcon name="eye" class="size-4" />
          </button>

          <!-- Refresh -->
          <button class="p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800" title="Refresh" @click="filesStore.refresh()">
            <AppIcon name="refresh" class="size-4" />
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
          <AppIcon name="upload" class="size-12 text-blue-400" :stroke-width="1.5" />
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
        @drop-to-folder="onDropToFolder"
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
        @drop-to-folder="onDropToFolder"
      />
    </div>

    <FilePreview
      :entry="previewEntry"
      :has-prev="previewIndex > 0"
      :has-next="previewIndex < previewableEntries.length - 1"
      :index="previewIndex"
      :total="previewableEntries.length"
      :text-edit-mode="previewTextEditMode"
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
            <AppButton @click="newFolderDialog = false">Cancel</AppButton>
            <AppButton variant="primary" @click="confirmNewFolder">Create</AppButton>
          </div>
        </div>
      </div>
    </Transition>

    <!-- New text file dialog -->
    <Transition name="fade">
      <div
        v-if="newTextFileDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        @click.self="newTextFileDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">New text file</h3>
          <input
            ref="newTextFileInput"
            v-model="newTextFileName"
            type="text"
            placeholder="filename.txt"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
            @keydown.enter="confirmNewTextFile"
            @keydown.escape="newTextFileDialog = false"
          />
          <div class="flex gap-2 mt-3 justify-end">
            <AppButton @click="newTextFileDialog = false">Cancel</AppButton>
            <AppButton variant="primary" @click="confirmNewTextFile">Create</AppButton>
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
            <AppButton @click="cancelRename">Cancel</AppButton>
            <AppButton variant="primary" @click="confirmRename">Rename</AppButton>
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
          <p class="text-xs text-zinc-400 mb-3 break-all">
            <template v-if="deletePending && deletePending.paths.length === 1">
              This will delete <span class="text-zinc-200">{{ deletePending.name ?? 'this file' }}</span> from your files.
            </template>
            <template v-else>
              This will delete {{ deletePending?.paths.length }} items from your files.
            </template>
          </p>
          <div class="flex gap-2 mt-3 justify-end">
            <AppButton @click="cancelDelete">Cancel</AppButton>
            <AppButton variant="destructive" @click="confirmDelete">Delete</AppButton>
          </div>
        </div>
      </div>
    </Transition>

    <ConflictDialog />

    <!-- Compress dialog -->
    <Transition name="fade">
      <div
        v-if="compressDialog"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
        @click.self="compressDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-80 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">Compress</h3>

          <label class="block text-xs text-zinc-400 mb-1">Archive name</label>
          <input
            v-model="compressName"
            type="text"
            placeholder="archive.zip"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors mb-3"
            @keydown.enter="confirmCompress"
            @keydown.escape="compressDialog = false"
          />

          <label class="block text-xs text-zinc-400 mb-1">Format</label>
          <AppSelect v-model="compressFormat" :options="compressFormatOptions" class="mb-3" />

          <label class="block text-xs text-zinc-400 mb-1">
            Password
            <span class="text-zinc-600">(optional{{ compressFormat === 'tar' || compressFormat === 'tar.gz' ? ', not supported for TAR' : '' }})</span>
          </label>
          <input
            v-model="compressPassword"
            type="password"
            placeholder="No password"
            :disabled="compressFormat === 'tar' || compressFormat === 'tar.gz'"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors disabled:opacity-40"
            @keydown.enter="confirmCompress"
            @keydown.escape="compressDialog = false"
          />

          <div v-if="compressFormat === '7z' && compressPassword" class="flex items-center gap-2 mt-3">
            <AppCheckbox v-model="compressProtectFilenames" />
            <span class="text-xs text-zinc-300">Encrypt filenames</span>
          </div>

          <div class="flex gap-2 mt-3 justify-end">
            <AppButton @click="compressDialog = false">Cancel</AppButton>
            <AppButton variant="primary" :disabled="!compressName.trim()" @click="confirmCompress">Create</AppButton>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Extract password dialog -->
    <Transition name="fade">
      <div
        v-if="extractPasswordDialog"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
        @click.self="extractPasswordDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-1">Password required</h3>
          <p class="text-xs text-zinc-400 mb-3">This archive is password protected.</p>
          <input
            v-model="extractPasswordInput"
            type="password"
            placeholder="Enter password"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
            @keydown.enter="confirmExtract"
            @keydown.escape="extractPasswordDialog = false"
          />
          <p v-if="extractError" class="text-xs text-red-400 mt-2">{{ extractError }}</p>
          <div class="flex gap-2 mt-3 justify-end">
            <AppButton @click="extractPasswordDialog = false">Cancel</AppButton>
            <AppButton variant="primary" :disabled="extracting || !extractPasswordInput" @click="confirmExtract">{{ extracting ? 'Extracting…' : 'Extract' }}</AppButton>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Background jobs card (upload progress + zip download) -->
    <Transition name="slide-up">
      <div
        v-if="bgJobs.length > 0"
        class="fixed bottom-6 right-6 z-[9998] bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl p-4 w-72 space-y-3"
      >
        <template v-for="(job, i) in bgJobs" :key="job.id">
          <div v-if="i > 0" class="border-t border-zinc-800" />

          <!-- Zip download -->
          <div v-if="job.kind === 'zip'" class="flex items-center gap-3">
            <AppIcon name="archive" class="size-4 text-blue-400 shrink-0 animate-pulse" />
            <span class="text-sm font-medium text-zinc-100">Zipping files…</span>
          </div>

          <!-- Compress -->
          <div v-else-if="job.kind === 'compress'" class="flex items-center gap-3">
            <AppIcon name="archive" class="size-4 text-blue-400 shrink-0 animate-pulse" />
            <span class="text-sm font-medium text-zinc-100 truncate">Creating {{ job.name }}…</span>
          </div>

          <!-- Upload -->
          <div v-else>
            <div class="flex items-center justify-between mb-2">
              <span class="text-sm font-medium text-zinc-100">
                Uploading {{ job.count }} file{{ job.count !== 1 ? 's' : '' }}…
              </span>
              <div class="flex items-center gap-2">
                <span class="text-xs text-zinc-400">{{ job.pct }}%</span>
                <button
                  class="text-zinc-500 hover:text-zinc-200 transition-colors"
                  title="Cancel upload"
                  @click="job.abort()"
                ><AppIcon name="x" class="size-3.5" /></button>
              </div>
            </div>
            <div class="h-1.5 bg-zinc-700 rounded-full overflow-hidden">
              <div
                class="h-full bg-blue-500 rounded-full transition-[width] duration-200"
                :style="{ width: `${job.pct}%` }"
              />
            </div>
            <div v-if="job.total > 0" class="mt-1.5 flex items-center justify-between text-xs text-zinc-500">
              <span>{{ formatEta(job) }}</span>
              <span>{{ formatSize(job.loaded) }} / {{ formatSize(job.total) }}</span>
            </div>
          </div>
        </template>
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
              <dd class="text-xs text-zinc-200 break-all">{{ detailsItems[0]?.name }}</dd>
            </div>

            <!-- Location -->
            <div class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Location</dt>
              <dd class="text-xs text-zinc-400 break-all font-mono">{{ detailsLocation }}</dd>
            </div>

            <!-- Type (single only) -->
            <div v-if="detailsItems.length === 1" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Type</dt>
              <dd class="text-xs text-zinc-200">{{ detailsItems[0]?.is_dir ? 'Folder' : (detailsItems[0]?.mime || 'Unknown') }}</dd>
            </div>

            <!-- Size -->
            <div class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Size</dt>
              <dd class="text-xs text-zinc-200">
                <!-- Single file: show directly -->
                <template v-if="detailsItems.length === 1 && !detailsItems[0]?.is_dir">
                  {{ formatSize(detailsItems[0]?.size ?? 0) }}
                </template>
                <!-- Streaming (folder or multi) -->
                <template v-else>{{ detailsSizeText }}</template>
              </dd>
            </div>

            <!-- Created (single only, if filesystem provides it) -->
            <div v-if="detailsItems.length === 1 && detailsItems[0]?.created_at" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Created</dt>
              <dd class="text-xs text-zinc-200">{{ formatDate(detailsItems[0]?.created_at ?? '') }}</dd>
            </div>

            <!-- Modified (single only) -->
            <div v-if="detailsItems.length === 1" class="flex gap-2">
              <dt class="text-xs text-zinc-500 w-20 shrink-0">Modified</dt>
              <dd class="text-xs text-zinc-200">{{ formatDate(detailsItems[0]?.mod_time ?? '') }}</dd>
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
            <AppButton @click="closeDetails">Close</AppButton>
          </div>
        </div>
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

.slide-up-enter-active,
.slide-up-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
