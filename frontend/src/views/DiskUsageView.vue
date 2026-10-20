<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { SizeMeasure, UsageEntry } from '@/types'
import { useDiskUsage } from '@/composables/useDiskUsage'
import { useHeaderCenter } from '@/composables/useHeaderCenter'
import { useToast } from '@/composables/useToast'
import { filesLocation, folderSize, sizeOf, sortBySize, usageQuery } from '@/utils/diskUsage'
import { formatSize, formatTimeAgo } from '@/utils/formatters'
import { isModalOpen, isTypingTarget } from '@/utils/keyboard'
import { isWithin, parentOf } from '@/utils/paths'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import UsageList from '@/components/usage/UsageList.vue'
import UsageTreemap from '@/components/usage/UsageTreemap.vue'

type Tab = 'contents' | 'largest'

const route = useRoute()
const router = useRouter()
const { show: showToast } = useToast()

// Everything the page shows is in the URL: ?path= the scanned folder, ?at= the
// folder looked at inside it, ?tab=largest, ?size=apparent.
const root = computed(() => (route.query.path as string | undefined) ?? '')
const at = computed(() => {
  const folder = route.query.at as string | undefined
  return folder !== undefined && isWithin(root.value, folder) ? folder : root.value
})
const tab = computed<Tab>(() => (route.query.tab === 'largest' ? 'largest' : 'contents'))
const measure = computed<SizeMeasure>(() => (route.query.size === 'apparent' ? 'apparent' : 'disk'))

function usageLocation(changes: { root?: string; at?: string; tab?: Tab; measure?: SizeMeasure }) {
  const state = { root: root.value, at: at.value, tab: tab.value, measure: measure.value, ...changes }
  return { name: 'usage', query: usageQuery(state) }
}

function go(changes: Parameters<typeof usageLocation>[0]) {
  router.push(usageLocation(changes))
}

const { listing, largest, loading, error, scanning, queued, progress, rescan, cancelScan, loadLargest, remove } = useDiskUsage(root, at)

const entries = computed(() => (listing.value ? sortBySize(listing.value.entries, measure.value) : []))
// What the bars are a part of: the folder looked at, or the whole scan for its
// largest files.
const folderTotal = computed(() => (listing.value ? folderSize(listing.value.total, entries.value, measure.value) : 0))
const rootTotal = computed(() => (listing.value ? sizeOf(listing.value.root_total, measure.value) : 0))
const shownEntries = computed(() => (tab.value === 'largest' ? largest.value ?? [] : entries.value))

watch([tab, measure, listing], () => {
  if (tab.value === 'largest' && listing.value) loadLargest(measure.value)
}, { immediate: true })

// ── Selection and deleting ────────────────────────────────────────────────
const selectedPaths = ref<string[]>([])
watch([at, tab, root], () => { selectedPaths.value = [] })
const selectedEntries = computed(() => shownEntries.value.filter((entry) => selectedPaths.value.includes(entry.path)))
const selectedSize = computed(() => selectedEntries.value.reduce((sum, entry) => sum + sizeOf(entry, measure.value), 0))

const deleteTargets = ref<UsageEntry[]>([])
const deleting = ref(false)
const deleteSize = computed(() => deleteTargets.value.reduce((sum, entry) => sum + sizeOf(entry, measure.value), 0))

function askDelete(targets: UsageEntry[]) {
  if (targets.length) deleteTargets.value = targets
}

async function confirmDelete() {
  const paths = deleteTargets.value.map((entry) => entry.path)
  deleting.value = true
  try {
    const freed = await remove(paths)
    showToast(`Freed ${formatSize(sizeOf(freed, measure.value))}`, 'success')
    selectedPaths.value = selectedPaths.value.filter((path) => !paths.includes(path))
  } catch (err) {
    showToast((err as Error).message, 'error')
  } finally {
    deleting.value = false
    deleteTargets.value = []
  }
}

// ── Moving around ─────────────────────────────────────────────────────────
function openFolder(entry: UsageEntry) {
  go({ at: entry.path, tab: 'contents' })
}

function reveal(entry: UsageEntry) {
  router.push({ name: 'files', query: filesLocation(entry) })
}

function backToFiles() {
  cancelScan()
  router.push({ name: 'files', query: at.value ? { path: at.value } : {} })
}

function toggleSelect(entry: UsageEntry) {
  selectedPaths.value = selectedPaths.value.includes(entry.path)
    ? selectedPaths.value.filter((path) => path !== entry.path)
    : [...selectedPaths.value, entry.path]
}

// The breadcrumb starts at the scanned folder; a typed path outside it is a
// new folder to look at.
useHeaderCenter('UsageBreadcrumb', () => h(FileBreadcrumb, {
  path: at.value,
  root: root.value,
  hrefFor: (path: string) => router.resolve(usageLocation({ at: path, tab: 'contents' })).href,
  onNavigate: (path: string) => (isWithin(root.value, path) ? go({ at: path, tab: 'contents' }) : go({ root: path, at: path, tab: 'contents' })),
}))

// Backspace goes up a folder, like in the file browser.
function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Backspace' || isTypingTarget(event.target) || isModalOpen()) return
  if (at.value === root.value || tab.value !== 'contents') return
  event.preventDefault()
  go({ at: parentOf(at.value) })
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

function plural(count: number, word: string): string {
  return `${count.toLocaleString()} ${word}${count !== 1 ? 's' : ''}`
}
</script>

<template>
  <div class="h-full overflow-auto">
    <div class="max-w-5xl mx-auto px-4 sm:px-6 py-4 sm:py-6 space-y-4">
      <!-- Title, totals and controls -->
      <div class="flex flex-col sm:flex-row sm:items-center gap-3 sm:gap-4">
        <div class="min-w-0 flex-1">
          <h1 class="flex items-center gap-2 text-lg font-semibold text-white">
            <AppIcon name="chartPie" class="size-5 text-blue-500" />
            Disk usage
          </h1>
          <p v-if="listing" class="mt-0.5 text-xs text-zinc-500">
            {{ formatSize(folderTotal) }} in {{ plural(listing.total.files, 'file') }}, {{ plural(listing.total.dirs, 'folder') }}
            · scanned {{ formatTimeAgo(listing.scanned_at) }}
            <span v-if="listing.unreadable" class="text-amber-400"> · {{ plural(listing.unreadable, 'folder') }} couldn't be read</span>
          </p>
        </div>
        <div class="flex items-center gap-2">
          <div class="flex rounded-lg bg-zinc-900 border border-zinc-800 p-0.5 text-xs" role="group" aria-label="Size">
            <button
              v-for="option in ([['disk', 'On disk'], ['apparent', 'File sizes']] as const)"
              :key="option[0]"
              type="button"
              :class="['px-2.5 py-1 rounded-md transition-colors', measure === option[0] ? 'bg-zinc-700 text-white' : 'text-zinc-400 hover:text-white']"
              :aria-pressed="measure === option[0]"
              @click="go({ measure: option[0] })"
            >{{ option[1] }}</button>
          </div>
          <AppButton variant="secondary" :disabled="scanning" @click="rescan">
            <span class="flex items-center gap-1.5"><AppIcon name="refresh" class="size-4" />Rescan</span>
          </AppButton>
        </div>
      </div>

      <!-- Scanning -->
      <div v-if="scanning" class="rounded-xl bg-zinc-900 border border-zinc-800 p-6 flex flex-col items-center gap-3 text-center">
        <AppSpinner size="lg" class="text-blue-500" />
        <p class="text-sm text-zinc-200">{{ queued ? 'Waiting for another scan to finish…' : 'Scanning…' }}</p>
        <p v-if="progress" class="text-xs text-zinc-500 tabular-nums">
          {{ plural(progress.files, 'file') }} · {{ plural(progress.dirs, 'folder') }} · {{ formatSize(sizeOf(progress, measure)) }}
        </p>
        <AppButton @click="backToFiles">Cancel</AppButton>
      </div>

      <!-- Couldn't scan or load -->
      <div v-else-if="error" class="rounded-xl bg-zinc-900 border border-zinc-800 p-6 flex flex-col items-center gap-3 text-center">
        <p class="text-sm text-red-400">{{ error }}</p>
        <div class="flex gap-2">
          <AppButton @click="backToFiles">Back to files</AppButton>
          <AppButton variant="primary" @click="rescan">Scan again</AppButton>
        </div>
      </div>

      <div v-else-if="!listing && loading" class="flex justify-center py-12 text-zinc-500"><AppSpinner /></div>

      <template v-else-if="listing">
        <UsageTreemap
          v-if="tab === 'contents' && folderTotal > 0"
          class="h-48 sm:h-72"
          :entries="entries"
          :measure="measure"
          :total="folderTotal"
          :selected="selectedPaths"
          @open="openFolder"
          @select="toggleSelect"
        />

        <!-- Tabs and the selection -->
        <div class="flex flex-wrap items-center gap-2 border-b border-zinc-800">
          <button
            v-for="option in ([['contents', 'Contents'], ['largest', 'Largest files']] as const)"
            :key="option[0]"
            type="button"
            :class="['px-3 py-2 -mb-px text-sm border-b-2 transition-colors', tab === option[0] ? 'border-blue-500 text-white' : 'border-transparent text-zinc-400 hover:text-white']"
            @click="go({ tab: option[0] })"
          >{{ option[1] }}</button>
          <div class="flex-1" />
          <div v-if="selectedEntries.length" class="flex items-center gap-2 pb-1">
            <span class="text-xs text-zinc-400">{{ selectedEntries.length }} selected · {{ formatSize(selectedSize) }}</span>
            <AppButton @click="selectedPaths = []">Clear</AppButton>
            <AppButton variant="destructive" @click="askDelete(selectedEntries)">Delete</AppButton>
          </div>
        </div>

        <div v-if="tab === 'largest' && !largest" class="flex justify-center py-8 text-zinc-500"><AppSpinner /></div>
        <p v-else-if="!shownEntries.length" class="py-8 text-center text-sm text-zinc-500">
          {{ tab === 'largest' ? 'No files found.' : 'This folder is empty.' }}
        </p>
        <UsageList
          v-else
          v-model:selected="selectedPaths"
          :entries="shownEntries"
          :measure="measure"
          :total="tab === 'largest' ? rootTotal : folderTotal"
          :show-paths="tab === 'largest'"
          :root="root"
          @open="openFolder"
          @reveal="reveal"
          @delete="(entry) => askDelete([entry])"
        />
      </template>
    </div>

    <AppDialog :open="deleteTargets.length > 0" width="w-80 max-w-[calc(100vw-2rem)]" @close="deleteTargets = []">
      <h3 class="text-sm font-semibold text-zinc-100 mb-3">
        {{ deleteTargets.length === 1 ? `Delete ${deleteTargets[0]!.is_dir && !deleteTargets[0]!.link ? 'folder' : 'file'}?` : `Delete ${deleteTargets.length} items?` }}
      </h3>
      <p class="text-xs text-zinc-400 mb-3">
        <template v-if="deleteTargets.length === 1"><span class="text-zinc-200 break-all">{{ deleteTargets[0]!.name }}</span> ({{ formatSize(deleteSize) }})</template>
        <template v-else>{{ formatSize(deleteSize) }} in total</template>
        will be deleted for good. This can't be undone.
      </p>
      <div class="flex justify-end gap-2">
        <AppButton @click="deleteTargets = []">Cancel</AppButton>
        <AppButton variant="destructive" :disabled="deleting" @click="confirmDelete">Delete</AppButton>
      </div>
    </AppDialog>
  </div>
</template>
