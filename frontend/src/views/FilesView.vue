<script setup lang="ts">
import { ref, computed, defineComponent, h, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useFilesStore } from '@/stores/files'
import type { FileEntry } from '@/types'
import { fileCategory, formatSize } from '@/utils/formatters'
import type { IconName } from '@/utils/icons'
import { files as filesApi, folders as foldersApi, fileops, type ConflictMode } from '@/utils/api'
import FileDetailsModal from '@/components/files/FileDetailsModal.vue'
import { useToast } from '@/composables/useToast'
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
import AdvancedRenamer from '@/components/files/AdvancedRenamer.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { useArchiveExtract } from '@/composables/useArchiveExtract'
import { isTypingTarget } from '@/utils/keyboard'
import { droppedFiles } from '@/utils/drop'
import { useJobSelection } from '@/composables/useJobSelection'
import { useFileOps } from '@/composables/useFileOps'
import { useLayoutStore } from '@/stores/layout'
import { debounce } from '@/utils/debounce'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import SecretInput from '@/components/ui/SecretInput.vue'
const filesStore = useFilesStore()
const layoutStore = useLayoutStore()
const route = useRoute()
const router = useRouter()
const { prompt: promptConflict, runWithConflictPrompt } = useConflictDialog()
const { extractArchive } = useArchiveExtract()
const { show: showToast } = useToast()

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

const debouncedExecuteSearch = debounce((query: string) => {
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

function startSearch(query: string) {
  debouncedExecuteSearch.cancel()
  closeSearchSource()
  searchResults.value = []
  searchLoading.value = true
  debouncedExecuteSearch(query)
}

watch(searchQuery, (query) => {
  if (((route.query.q as string) ?? '') !== query) {
    router.replace({ query: routeQuery() })
  }
  if (!query.trim()) {
    debouncedExecuteSearch.cancel()
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
  debouncedExecuteSearch.cancel()
  closeSearchSource()
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
const previewRawText      = ref(false) // custom-view files (.todo, .kanban) shown as plain text; ?view=text
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
const prevPreviewEntry = computed<FileEntry | null>(
  () => previewableEntries.value[previewIndex.value - 1] ?? null,
)
const nextPreviewEntry = computed<FileEntry | null>(
  () => previewableEntries.value[previewIndex.value + 1] ?? null,
)

// Every way of opening a file in the preview goes through here, so per-file view
// modes reset when moving to another file.
function showPreview(path: string, { editMode = false, rawText = false } = {}) {
  previewTextEditMode.value = editMode
  previewRawText.value = rawText
  previewFilePath.value = path
}

function openPreview(entry: FileEntry) { showPreview(entry.path) }
function openAsText(entry: FileEntry) { showPreview(entry.path, { rawText: true }) }
function closePreview() {
  previewRawText.value = false
  previewFilePath.value = null
}

function prevPreview() {
  const previousEntry = previewableEntries.value[previewIndex.value - 1]
  if (previousEntry) showPreview(previousEntry.path)
}
function nextPreview() {
  const nextEntry = previewableEntries.value[previewIndex.value + 1]
  if (nextEntry) showPreview(nextEntry.path)
}

// ── URL-synced navigation ──────────────────────────────────────────────────
function routeQuery() {
  const q: Record<string, string> = {}
  if (filesStore.currentPath) q.path = filesStore.currentPath
  if (searchQuery.value) q.q = searchQuery.value
  if (previewEntry.value) q.preview = previewEntry.value.path
  if (previewEntry.value && previewRawText.value) q.view = 'text'
  return q
}

async function navigate(path: string) {
  const actualPath = await filesStore.navigate(path)
  fetchFolderPref(actualPath)
  selectedPaths.value = []
  lastAnchorPath.value = null
  router.push({ query: actualPath ? { path: actualPath } : {} })
}

// The open folder was removed (seen by the live stream): go to its nearest
// existing parent.
watch(() => filesStore.folderGone, (fallback) => {
  if (fallback === null) return
  filesStore.folderGone = null
  showToast('This folder no longer exists', 'info')
  navigate(fallback)
})

watch(previewEntry, (entry) => {
  const currentPreview = route.query.preview as string | undefined
  if (entry?.path === currentPreview) return
  router.replace({ query: routeQuery() })
})

watch(previewRawText, (rawText) => {
  if (rawText === (route.query.view === 'text')) return
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
    previewRawText.value = q.view === 'text'
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

function refresh() {
  deselectAll()
  filesStore.refresh()
}

// ── Background jobs (upload progress, zip download) ────────────────────────
interface UploadJob   { kind: 'upload';   id: number; count: number; pct: number; loaded: number; total: number; startedAt: number; abort: () => void }
interface ZipJob      { kind: 'zip';      id: number }
interface CompressJob { kind: 'compress'; id: number; name: string }
type BgJob = UploadJob | ZipJob | CompressJob

const bgJobs = ref<BgJob[]>([])
let nextJobId = 0

const downloading = computed(() => bgJobs.value.some(j => j.kind === 'zip'))

// ── File ops (single source of truth for all selection-based operations) ────
const {
  clipboard,
  contextMenuState,
  onContextMenu,
  fileOps,
  contextMenuItems,
  onPaste,
  performMove,
} = useFileOps({
  selectedPaths,
  visibleEntries,
  searchResults,
  downloading,
  currentPath: computed(() => filesStore.currentPath),
  showToast,
  actionCallbacks: {
    downloadPaths,
    openCompressDialog,
    openExtract,
    startRename,
    openAdvancedRenamer,
    openDeleteDialog,
    openDetails,
    entriesForPaths,
    openAsText,
  },
})

// ── Selection-clear tracking for background jobs ────────────────────────────
const { trackJob, clearSelectionAfterJob } = useJobSelection(selectedPaths, () => filesStore.entries)

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
  trackJob(id)
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
    clearSelectionAfterJob(id)
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

function openFilePicker() {
  uploadInput.value?.click()
}

async function handleFileInput(e: Event) {
  const files = Array.from((e.target as HTMLInputElement).files ?? [])
  if (!files.length) return
  await uploadFiles(files)
  if (uploadInput.value) uploadInput.value.value = ''
}

// Asks first when names are taken in the folder as listed here; the server
// checks again, in case the folder changed since.
async function uploadFiles(files: File[]) {
  try {
    const known = detectConflicts(files)
    let uploaded
    if (known.length > 0) {
      const choice = await promptConflict('Upload conflicts', known)
      if (!choice) return
      uploaded = await performUpload(files, choice)
    } else {
      uploaded = await runWithConflictPrompt('Upload conflicts', (conflictMode) => performUpload(files, conflictMode))
      if (uploaded === null) return
    }
    showToast(`Uploaded ${uploaded.count} file${uploaded.count !== 1 ? 's' : ''}`, 'success')
  } catch (err) {
    if ((err as Error).message !== 'cancelled') showToast((err as Error).message, 'error')
  }
}

// Uploads into the current folder, shown in the background jobs list meanwhile.
async function performUpload(files: File[], conflictMode: ConflictMode) {
  const totalSize = files.reduce((s, f) => s + f.size, 0)
  const id = nextJobId++
  const { promise, abort } = fileops.upload(filesStore.currentPath, files, (loaded, total) => {
    const job = bgJobs.value.find(j => j.id === id) as UploadJob | undefined
    if (job) { job.pct = Math.round((loaded / total) * 100); job.loaded = loaded; job.total = total }
  }, conflictMode)
  bgJobs.value.push({ kind: 'upload', id, count: files.length, pct: 0, loaded: 0, total: totalSize, startedAt: Date.now(), abort })
  try {
    return await promise
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
  const { files, folders } = droppedFiles(e.dataTransfer)
  if (folders.length) {
    const skipped = folders.length === 1 ? `"${folders[0]}" was` : `${folders.length} folders were`
    showToast(`${skipped} left out: folders can't be uploaded`, 'info')
  }
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

// ── New file (text / todo / kanban) ─────────────────────────────────────────
interface NewFileConfig {
  title: string
  extension: string
  editMode: boolean // open the preview in edit mode (plain-text editor)
}
// Single source for the "new file" entries in the toolbar, mobile menu and folder context menu.
const newFileTypes: (NewFileConfig & { icon: IconName })[] = [
  { title: 'New text file',    extension: 'txt',    editMode: true,  icon: 'newFile' },
  { title: 'New todo file',    extension: 'todo',   editMode: false, icon: 'listChecks' },
  { title: 'New kanban board', extension: 'kanban', editMode: false, icon: 'kanban' },
]
const newFileDialog = ref(false)
const newFileName = ref('')
const newFileInput = ref<HTMLInputElement | null>(null)
const newFileConfig = ref<NewFileConfig>({ title: '', extension: 'txt', editMode: false })

async function openNewFile(config: NewFileConfig) {
  newFileConfig.value = config
  newFileName.value = ''
  newFileDialog.value = true
  await nextTick()
  newFileInput.value?.focus()
}

async function confirmNewFile() {
  const name = newFileName.value.trim()
  if (!name) return
  newFileDialog.value = false
  const { extension, editMode } = newFileConfig.value
  const filename = name.endsWith(`.${extension}`) ? name : `${name}.${extension}`
  await createFile(filename, editMode)
}

async function createFile(filename: string, editMode: boolean) {
  const folder = filesStore.currentPath
  const pathIn = (name: string) => (folder ? `${folder}/${name}` : name)
  try {
    const created = await runWithConflictPrompt('File already exists', (conflictMode) => fileops.create(pathIn(filename), conflictMode))
    if (created) showPreview(pathIn(created.name), { editMode })
  } catch (err) {
    showToast((err as Error).message, 'error')
  }
}

// ── Advanced rename ───────────────────────────────────────────────────────
const advancedRenamerOpen = ref(false)
const advancedRenamerFiles = ref<FileEntry[]>([])

function openAdvancedRenamer(paths: string[]) {
  const pool = [...visibleEntries.value, ...searchResults.value]
  advancedRenamerFiles.value = paths
    .map((path) => pool.find((entry) => entry.path === path))
    .filter((entry): entry is FileEntry => entry !== undefined)
  advancedRenamerOpen.value = true
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
    const renamed = await runWithConflictPrompt('Name already taken', (conflictMode) => fileops.rename(entry.path, trimmed, conflictMode))
    if (!renamed) return
    selectedPaths.value = []
    // Optimistically update the entry so preview index can be resolved immediately.
    // The watch stream will deliver the authoritative REMOVE + ADD events shortly after.
    const parent = entry.path.includes('/') ? entry.path.slice(0, entry.path.lastIndexOf('/')) : ''
    const newPath = parent ? `${parent}/${renamed.name}` : renamed.name
    const idx = filesStore.entries.findIndex((e) => e.path === entry.path)
    if (idx >= 0) {
      const existing = filesStore.entries[idx]!
      filesStore.entries[idx] = { ...existing, name: renamed.name, path: newPath }
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

async function runCompress(paths: string[], dest: string, format: string, password: string | undefined, protect: boolean) {
  const name = dest.split('/').pop() ?? dest
  const id = nextJobId++
  trackJob(id)
  bgJobs.value.push({ kind: 'compress', id, name })
  try {
    const created = await runWithConflictPrompt('Archive already exists', (conflictMode) =>
      fileops.compress(paths, dest, format, password, protect, conflictMode),
    )
    if (created === null) return
    clearSelectionAfterJob(id)
    showToast('Archive created', 'success')
  } catch (err) {
    showToast(err instanceof Error ? err.message : 'Failed to create archive', 'error')
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

async function runExtract(path: string, password: string | undefined) {
  try {
    if ((await extractArchive(path, password)) === null) return
    selectedPaths.value = []
    showToast('Extracted successfully', 'success')
  } catch (e) {
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
    const extracted = await extractArchive(extractPendingPath.value, extractPasswordInput.value)
    extractPasswordDialog.value = false
    if (extracted === null) return
    selectedPaths.value = []
    showToast('Extracted successfully', 'success')
  } catch (e) {
    extractError.value = e instanceof Error ? e.message : 'Extraction failed'
  } finally {
    extracting.value = false
  }
}

// ── Details panel ─────────────────────────────────────────────────────────
const detailsItems = ref<FileEntry[]>([])

function entriesForPaths(paths: string[]): FileEntry[] {
  const pool = [...visibleEntries.value, ...searchResults.value]
  return paths.map((p) => pool.find((e) => e.path === p)).filter((e): e is FileEntry => !!e)
}

function openDetails(items: FileEntry[]) {
  detailsItems.value = items
}

function closeDetails() {
  detailsItems.value = []
}

function onGlobalKeydown(event: KeyboardEvent) {
  if (!event.ctrlKey && !event.metaKey) return
  if (event.key !== 'a' && event.key !== 'A') return
  if (isTypingTarget(event.target)) return
  event.preventDefault()
  if (selectedPaths.value.length === visibleEntries.value.length && visibleEntries.value.length > 0) {
    deselectAll()
  } else {
    selectAll()
  }
}

onMounted(() => { window.addEventListener('keydown', onGlobalKeydown) })
onUnmounted(() => { window.removeEventListener('keydown', onGlobalKeydown) })

// ── Folder context menu (right-click on empty space) ──────────────────────
const folderContextMenuState = ref<{ x: number; y: number } | null>(null)

function onFolderContextMenu(x: number, y: number) {
  folderContextMenuState.value = { x, y }
}

const folderContextMenuItems = computed((): ContextMenuItem[] => {
  const close = () => { folderContextMenuState.value = null }
  return [
    ...newFileTypes.map((fileType) => ({
      label: fileType.title,
      icon: fileType.icon,
      action: () => { close(); openNewFile(fileType) },
    } satisfies ContextMenuItem)),
    {
      label: 'New folder',
      icon: 'newFolder',
      action: () => { close(); openNewFolder() },
    },
    { separator: true },
    ...(clipboard.value ? [{
      label: 'Paste',
      icon: 'clipboard',
      action: () => { close(); onPaste() },
    } satisfies ContextMenuItem] : []),
    {
      label: 'Upload files',
      icon: 'upload',
      action: () => { close(); openFilePicker() },
    },
    { separator: true },
    {
      label: 'Refresh',
      icon: 'refresh',
      action: () => { close(); refresh() },
    },
    {
      label: 'Details',
      icon: 'info',
      action: async () => {
        close()
        const entry = await filesApi.stat(filesStore.currentPath)
        openDetails([entry])
      },
    },
  ]
})


// ── Computed helpers ──────────────────────────────────────────────────────
const fileopsActive = computed(() => selectedPaths.value.length > 0 || clipboard.value !== null)

// ── Register a header center component via the layout store ───────────────
// defineComponent closes over this view's reactive state — AppHeader renders
// it generically with <component :is="..."> without knowing what it is.
layoutStore.headerBreadcrumbsActive = true
layoutStore.headerCenter = defineComponent({
  name: 'HeaderBreadcrumb',
  setup() {
    return () => h(FileBreadcrumb, {
      path: filesStore.currentPath,
      onNavigate: navigate,
      onDropToFolder: onDropToFolder,
    })
  },
})
onUnmounted(() => {
  layoutStore.headerBreadcrumbsActive = false
  layoutStore.headerCenter = null
})

onMounted(async () => {
  const initialPath = (route.query.path as string) || ''
  const actualPath = await filesStore.navigate(initialPath)
  fetchFolderPref(actualPath)
  if (actualPath !== initialPath) {
    router.replace({ query: actualPath ? { path: actualPath } : {} })
  }
  if (route.query.preview) previewFilePath.value = route.query.preview as string
  previewRawText.value = route.query.view === 'text'
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
    <!-- Toolbar -->
    <div class="border-b border-zinc-800 bg-zinc-900 shrink-0">

      <!-- Row: Search/FileOps (left, flex-1) + Action buttons (right, fixed) -->
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

        <!-- Right: action buttons — hidden on mobile when search or fileops active -->
        <div :class="['items-center gap-0.5 shrink-0', mobileSearchOpen || fileopsActive ? 'hidden sm:flex' : 'flex']">

          <!-- Search icon — mobile only, not shown during fileops -->
          <button
            v-if="!fileopsActive"
            class="sm:hidden p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800"
            title="Search"
            @click="openMobileSearch"
          >
            <AppIcon name="search" class="size-4" />
          </button>

          <!-- New text / todo / kanban file: desktop only -->
          <button
            v-for="fileType in newFileTypes"
            :key="fileType.extension"
            class="hidden sm:flex p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800"
            :title="fileType.title"
            @click="openNewFile(fileType)"
          >
            <AppIcon :name="fileType.icon" class="size-4" />
          </button>

          <!-- New folder: desktop only -->
          <button class="hidden sm:flex p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800" title="New folder" @click="openNewFolder">
            <AppIcon name="newFolder" class="size-4" />
          </button>

          <!-- Upload: desktop only -->
          <button :class="['hidden sm:flex p-1.5 rounded transition-colors', bgJobs.some(j => j.kind === 'upload') ? 'text-blue-400 animate-pulse' : 'text-zinc-400 hover:text-white hover:bg-zinc-800']" title="Upload files" @click="openFilePicker">
            <AppIcon name="upload" class="size-4" />
          </button>
          <input ref="uploadInput" type="file" multiple class="hidden" @change="handleFileInput" />

          <!-- View toggle: always visible -->
          <div v-if="!searchQuery" class="flex items-center bg-zinc-800 rounded-lg p-0.5 gap-0.5 ml-0.5">
            <button :class="['p-1.5 rounded-md transition-colors', viewMode === 'grid' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white']" title="Grid view" @click="setView('grid')">
              <AppIcon name="grid" class="size-4" />
            </button>
            <button :class="['p-1.5 rounded-md transition-colors', viewMode === 'list' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white']" title="List view" @click="setView('list')">
              <AppIcon name="bars" class="size-4" />
            </button>
          </div>

          <!-- Show/hide hidden files: desktop only -->
          <button :class="['hidden sm:flex p-1.5 rounded transition-colors hover:bg-zinc-800', showHidden ? 'text-blue-400 hover:text-blue-300' : 'text-zinc-400 hover:text-white']" :title="showHidden ? 'Hide hidden files' : 'Show hidden files'" @click="toggleHidden">
            <AppIcon name="eye" class="size-4" />
          </button>

          <!-- Refresh: desktop only -->
          <button class="hidden sm:flex p-1.5 rounded text-zinc-400 hover:text-white transition-colors hover:bg-zinc-800" title="Refresh" @click="refresh()">
            <AppIcon name="refresh" class="size-4" />
          </button>

          <!-- ••• mobile actions dropdown -->
          <AppDropdown class="sm:hidden" align="right">
            <template #trigger="{ toggle }">
              <button
                class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors"
                title="More actions"
                @click="toggle"
              >
                <AppIcon name="ellipsis" class="size-4" />
              </button>
            </template>
            <template #default>
              <button
                v-for="fileType in newFileTypes"
                :key="fileType.extension"
                class="w-full text-left px-3 py-2.5 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 flex items-center gap-2.5 transition-colors"
                @click="openNewFile(fileType)"
              >
                <AppIcon :name="fileType.icon" class="size-4 shrink-0" /> {{ fileType.title }}
              </button>
              <button class="w-full text-left px-3 py-2.5 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 flex items-center gap-2.5 transition-colors" @click="openNewFolder()">
                <AppIcon name="newFolder" class="size-4 shrink-0" /> New folder
              </button>
              <button class="w-full text-left px-3 py-2.5 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 flex items-center gap-2.5 transition-colors" @click="openFilePicker()">
                <AppIcon name="upload" class="size-4 shrink-0" /> Upload files
              </button>
              <div class="my-1 border-t border-zinc-700" />
              <button :class="['w-full text-left px-3 py-2.5 text-sm hover:bg-zinc-800 flex items-center gap-2.5 transition-colors', showHidden ? 'text-blue-400' : 'text-zinc-300 hover:text-white']" @click="toggleHidden()">
                <AppIcon name="eye" class="size-4 shrink-0" /> {{ showHidden ? 'Hide hidden files' : 'Show hidden files' }}
              </button>
              <button class="w-full text-left px-3 py-2.5 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 flex items-center gap-2.5 transition-colors" @click="refresh()">
                <AppIcon name="refresh" class="size-4 shrink-0" /> Refresh
              </button>
            </template>
          </AppDropdown>
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
        @folder-context-menu="onFolderContextMenu"
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
        @folder-context-menu="onFolderContextMenu"
        @drop-to-folder="onDropToFolder"
      />
    </div>

    <FilePreview
      :entry="previewEntry"
      :prev-entry="prevPreviewEntry"
      :next-entry="nextPreviewEntry"
      :has-prev="previewIndex > 0"
      :has-next="previewIndex < previewableEntries.length - 1"
      :index="previewIndex"
      :total="previewableEntries.length"
      :text-edit-mode="previewTextEditMode"
      :raw-text="previewRawText"
      @close="closePreview"
      @prev="prevPreview"
      @next="nextPreview"
      @rename="startRename"
      @delete="(e) => openDeleteDialog([e.path], e.name)"
      @toggle-raw-text="previewRawText = !previewRawText"
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
        @mousedown.self="newFolderDialog = false"
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

    <!-- New file dialog (text / todo) -->
    <Transition name="fade">
      <div
        v-if="newFileDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        @mousedown.self="newFileDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">{{ newFileConfig.title }}</h3>
          <input
            ref="newFileInput"
            v-model="newFileName"
            type="text"
            :placeholder="`filename.${newFileConfig.extension}`"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
            @keydown.enter="confirmNewFile"
            @keydown.escape="newFileDialog = false"
          />
          <div class="flex gap-2 mt-3 justify-end">
            <AppButton @click="newFileDialog = false">Cancel</AppButton>
            <AppButton variant="primary" @click="confirmNewFile">Create</AppButton>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Rename dialog -->
    <Transition name="fade">
      <div
        v-if="renameDialog"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
        @mousedown.self="cancelRename"
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
        @mousedown.self="cancelDelete"
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

    <AdvancedRenamer
      v-if="advancedRenamerOpen"
      :files="advancedRenamerFiles"
      :folder="filesStore.currentPath"
      @close="advancedRenamerOpen = false"
    />

    <!-- Compress dialog -->
    <Transition name="fade">
      <div
        v-if="compressDialog"
        class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
        @mousedown.self="compressDialog = false"
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
          <SecretInput
            v-model="compressPassword"
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
        @mousedown.self="extractPasswordDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-72 shadow-2xl">
          <h3 class="text-sm font-semibold text-zinc-100 mb-1">Password required</h3>
          <p class="text-xs text-zinc-400 mb-3">This archive is password protected.</p>
          <SecretInput
            v-model="extractPasswordInput"
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

    <!-- Context menu (file/folder entry) -->
    <ContextMenu
      v-if="contextMenuState"
      :x="contextMenuState.x"
      :y="contextMenuState.y"
      :items="contextMenuItems"
      @close="contextMenuState = null"
    />

    <!-- Context menu (empty folder space) -->
    <ContextMenu
      v-if="folderContextMenuState"
      :x="folderContextMenuState.x"
      :y="folderContextMenuState.y"
      :items="folderContextMenuItems"
      @close="folderContextMenuState = null"
    />

    <!-- Details modal -->
    <FileDetailsModal :items="detailsItems" @close="closeDetails" />

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
