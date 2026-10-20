<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { UsageEntry } from '@/types'
import { useDiskUsage } from '@/composables/useDiskUsage'
import { useToast } from '@/composables/useToast'
import { filesLocation, folderSize, sortBySize, totalSize, withUsageState, type UsageState } from '@/utils/diskUsage'
import { formatSize, formatTimeAgo } from '@/utils/formatters'
import { isTypingTarget, topModal } from '@/utils/keyboard'
import { isWithin, parentOf } from '@/utils/paths'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AppLoading from '@/components/ui/AppLoading.vue'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import UsageList from '@/components/usage/UsageList.vue'
import UsageTreemap from '@/components/usage/UsageTreemap.vue'

// What takes the space in a folder, over the file browser. Everything it
// shows is in the URL, next to the browser's own keys (withUsageState).
const props = defineProps<{ state: UsageState }>()

const route = useRoute()
const router = useRouter()
const { show: showToast } = useToast()

const root = computed(() => props.state.root)
const at = computed(() => props.state.at)
const tab = computed(() => props.state.tab)

function usageLocation(changes: Partial<UsageState>) {
  return { query: withUsageState(route.query, { ...props.state, ...changes }) }
}

function go(changes: Partial<UsageState>) {
  router.push(usageLocation(changes))
}

// Leaving stops a scan still running (the modal's scope ends).
function close() {
  router.push({ query: withUsageState(route.query, null) })
}

const { listing, largest, loading, error, scanning, queued, progress, rescan, loadLargest, remove } = useDiskUsage(root, at)

const entries = computed(() => (listing.value ? sortBySize(listing.value.entries) : []))
// What the bars are a part of: the folder looked at, or the whole scan for its
// largest files.
const folderTotal = computed(() => (listing.value ? folderSize(listing.value.total, entries.value) : 0))
const rootTotal = computed(() => (listing.value ? listing.value.root_total.disk : 0))
const shownEntries = computed(() => (tab.value === 'largest' ? largest.value ?? [] : entries.value))
// A folder's listing still on screen while the next one loads.
const stale = computed(() => loading.value && listing.value !== null && listing.value.at !== at.value)

watch([tab, listing], () => {
  if (tab.value === 'largest' && listing.value) loadLargest()
}, { immediate: true })

// ── Selection and deleting ────────────────────────────────────────────────
const selectedPaths = ref<string[]>([])
watch([at, tab, root], () => { selectedPaths.value = [] })
const selectedEntries = computed(() => shownEntries.value.filter((entry) => selectedPaths.value.includes(entry.path)))
const selectedSize = computed(() => totalSize(selectedEntries.value))

const deleteTargets = ref<UsageEntry[]>([])
const deleting = ref(false)
const deleteSize = computed(() => totalSize(deleteTargets.value))

function askDelete(targets: UsageEntry[]) {
  if (targets.length) deleteTargets.value = targets
}

async function confirmDelete() {
  const paths = deleteTargets.value.map((entry) => entry.path)
  deleting.value = true
  try {
    const freed = await remove(paths)
    showToast(`Freed ${formatSize(freed.disk)}`, 'success')
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

// Closes the modal on the item in the file browser.
function reveal(entry: UsageEntry) {
  router.push({ query: filesLocation(entry) })
}

function toggleSelect(entry: UsageEntry) {
  selectedPaths.value = selectedPaths.value.includes(entry.path)
    ? selectedPaths.value.filter((path) => path !== entry.path)
    : [...selectedPaths.value, entry.path]
}

// The breadcrumb starts at the scanned folder; a typed path outside it is a
// new folder to scan.
function crumbHref(path: string): string {
  return router.resolve(usageLocation({ at: path, tab: 'contents' })).href
}

function onCrumbNavigate(path: string) {
  if (isWithin(root.value, path)) go({ at: path, tab: 'contents' })
  else go({ root: path, at: path, tab: 'contents' })
}

// Backspace goes up a folder, unless a dialog is open over the modal.
const content = ref<HTMLElement | null>(null)
function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Backspace' || isTypingTarget(event.target)) return
  if (!content.value || !topModal()?.contains(content.value)) return
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
  <AppModal title="Disk usage" size="xl" close-on-escape fixed-height @close="close">
    <div ref="content" class="space-y-4">
      <!-- Where: the breadcrumb starts at the scanned folder -->
      <div class="h-10 flex items-center rounded-lg bg-zinc-950/60 border border-zinc-800 px-1">
        <FileBreadcrumb :path="at" :root="root" :href-for="crumbHref" @navigate="onCrumbNavigate" />
      </div>

      <!-- Totals and controls -->
      <div class="flex items-center gap-3 sm:gap-4">
        <p class="min-w-0 flex-1 text-xs text-zinc-500" data-usage-summary>
          <template v-if="listing">
            {{ formatSize(folderTotal) }} in {{ plural(listing.total.files, 'file') }}, {{ plural(listing.total.dirs, 'folder') }}
            · scanned {{ formatTimeAgo(listing.scanned_at) }}
            <span v-if="listing.unreadable" class="text-amber-400"> · {{ plural(listing.unreadable, 'folder') }} couldn't be read</span>
          </template>
        </p>
        <AppButton variant="secondary" class="shrink-0" :disabled="scanning" @click="rescan">
          <span class="flex items-center gap-1.5"><AppIcon name="refresh" class="size-4" />Rescan</span>
        </AppButton>
      </div>

      <!-- Scanning -->
      <AppLoading
        v-if="scanning"
        :label="queued ? 'Waiting for another scan to finish…' : 'Scanning…'"
        class="rounded-xl bg-zinc-900 border border-zinc-800 px-6"
      >
        <p v-if="progress" class="text-xs text-zinc-500 tabular-nums">
          {{ plural(progress.files, 'file') }} · {{ plural(progress.dirs, 'folder') }} · {{ formatSize(progress.disk) }}
        </p>
        <AppButton @click="close">Cancel</AppButton>
      </AppLoading>

      <!-- Couldn't scan or load -->
      <div v-else-if="error" class="rounded-xl bg-zinc-900 border border-zinc-800 p-6 flex flex-col items-center gap-3 text-center">
        <p class="text-sm text-red-400">{{ error }}</p>
        <div class="flex gap-2">
          <AppButton @click="close">Close</AppButton>
          <AppButton variant="primary" @click="rescan">Scan again</AppButton>
        </div>
      </div>

      <AppLoading v-else-if="!listing && loading" label="Loading the folder…" />

      <div v-else-if="listing" :class="['space-y-4 transition-opacity', stale && 'opacity-50']">
        <UsageTreemap
          v-if="tab === 'contents' && folderTotal > 0"
          class="h-48 sm:h-72"
          :entries="entries"
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

        <AppLoading v-if="tab === 'largest' && !largest" label="Finding the largest files…" />
        <p v-else-if="!shownEntries.length" class="py-8 text-center text-sm text-zinc-500">
          {{ tab === 'largest' ? 'No files found.' : 'This folder is empty.' }}
        </p>
        <UsageList
          v-else
          v-model:selected="selectedPaths"
          :entries="shownEntries"
          :total="tab === 'largest' ? rootTotal : folderTotal"
          :show-paths="tab === 'largest'"
          :root="root"
          @open="openFolder"
          @reveal="reveal"
          @delete="(entry) => askDelete([entry])"
        />
      </div>

      <AppDialog :open="deleteTargets.length > 0" width="w-80 max-w-[calc(100vw-2rem)]" @close="deleteTargets = []">
        <h3 class="text-sm font-semibold text-zinc-100 mb-3">
          {{ deleteTargets.length === 1 ? `Delete ${deleteTargets[0]!.is_dir && !deleteTargets[0]!.link ? 'folder' : 'file'}?` : `Delete ${deleteTargets.length} items?` }}
        </h3>
        <p class="text-xs text-zinc-400">
          <template v-if="deleteTargets.length === 1"><span class="text-zinc-200 break-all">{{ deleteTargets[0]!.name }}</span> ({{ formatSize(deleteSize) }})</template>
          <template v-else>{{ formatSize(deleteSize) }} in total</template>
          will be deleted for good. This can't be undone.
        </p>
        <template #actions>
          <AppButton @click="deleteTargets = []">Cancel</AppButton>
          <AppButton variant="destructive" :disabled="deleting" @click="confirmDelete">Delete</AppButton>
        </template>
      </AppDialog>
    </div>
  </AppModal>
</template>
