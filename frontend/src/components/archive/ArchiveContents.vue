<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { files as filesApi } from '@/utils/api'
import { formatSize } from '@/utils/formatters'
import { archiveTree, searchArchive, withArchiveState, type ArchiveItem, type ArchiveState, type ArchiveTree } from '@/utils/archiveTree'
import { nameOf, parentOf, relativeTo } from '@/utils/paths'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import SecretInput from '@/components/ui/SecretInput.vue'
import { useArchiveExtract } from '@/composables/useArchiveExtract'

// What's in an archive, over the file browser. The archive and the folder
// looked at inside it are in the URL, next to the browser's own keys
// (withArchiveState).
const props = defineProps<{ state: ArchiveState }>()

const route = useRoute()
const router = useRouter()

const name = computed(() => nameOf(props.state.path))
const tree = ref<ArchiveTree>(archiveTree([]))
const loading = ref(true)
const error = ref('')
const password = ref('')
const passwordRequired = ref(false)
const search = ref('')
// The folder inside the archive being looked at ('' = its top); one the
// archive doesn't have shows the top.
const folder = computed(() => (tree.value.folders.has(props.state.at) ? props.state.at : ''))

const searching = computed(() => search.value.trim() !== '')
const shown = computed(() =>
  searching.value ? searchArchive(tree.value.items, folder.value, search.value) : tree.value.folders.get(folder.value) ?? [],
)

watch(folder, () => { search.value = '' })

function folderLocation(path: string) {
  return { query: withArchiveState(route.query, { ...props.state, at: path }) }
}

function openFolder(path: string) {
  router.push(folderLocation(path))
}

function crumbHref(path: string): string {
  return router.resolve(folderLocation(path)).href
}

function close() {
  router.push({ query: withArchiveState(route.query, null) })
}

// A search result opens itself if it's a folder, else the folder holding it.
function openItem(item: ArchiveItem) {
  if (item.is_dir) openFolder(item.path)
  else if (searching.value) openFolder(parentOf(item.path))
}

// Where a search result is, seen from the folder searched.
function resultLocation(item: ArchiveItem): string {
  return relativeTo(folder.value, parentOf(item.path))
}

const extracting = ref(false)
const extractError = ref('')
const extractSuccess = ref(false)
const extractPassword = ref('')
const { extractArchive } = useArchiveExtract()

async function load(pw = '') {
  loading.value = true
  error.value = ''
  try {
    tree.value = archiveTree(await filesApi.archiveContents(props.state.path, pw))
    passwordRequired.value = false
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : 'Failed to read archive'
    if (msg.toLowerCase().includes('password') || msg.toLowerCase().includes('encrypted')) {
      passwordRequired.value = true
      error.value = 'This archive is password protected'
    } else {
      error.value = msg
    }
  } finally {
    loading.value = false
  }
}

async function unlock() {
  if (!password.value) return
  await load(password.value)
}

async function extract() {
  extracting.value = true
  extractError.value = ''
  extractSuccess.value = false
  const pw = password.value || extractPassword.value || undefined
  try {
    if ((await extractArchive(props.state.path, pw)) === null) return
    extractSuccess.value = true
    close()
  } catch (e: unknown) {
    extractError.value = e instanceof Error ? e.message : 'Extraction failed'
  } finally {
    extracting.value = false
  }
}

onMounted(() => load())
</script>

<template>
  <AppModal :title="name" size="lg" @close="close">
    <!-- Password prompt -->
    <div v-if="passwordRequired" class="space-y-3">
      <p class="text-sm text-zinc-400">{{ error }}</p>
      <div class="flex gap-2">
        <SecretInput
          v-model="password"
          placeholder="Enter password"
          class="flex-1 bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
          @keydown.enter="unlock"
        />
        <button
          class="bg-blue-600 hover:bg-blue-700 text-white text-sm px-4 py-2 rounded-lg transition-colors"
          @click="unlock"
        >
          Unlock
        </button>
      </div>
    </div>

    <!-- Loading -->
    <div v-else-if="loading" class="flex justify-center py-8">
      <AppSpinner size="lg" class="text-blue-500" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2">
      {{ error }}
    </div>

    <!-- Contents -->
    <div v-else>
      <div class="flex items-center gap-2 mb-2 -ml-2 min-w-0">
        <FileBreadcrumb :path="folder" :root-label="name" :href-for="crumbHref" @navigate="openFolder" />
      </div>
      <!-- Search -->
      <div class="relative mb-2">
        <AppIcon name="search" class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none" />
        <input
          v-model="search"
          type="text"
          autocomplete="off"
          :placeholder="folder ? 'Search this folder…' : 'Search the archive…'"
          class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg pl-8 pr-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
          @keydown.escape="search = ''"
        />
      </div>
      <p class="text-xs text-zinc-500 mb-2">
        <template v-if="searching">{{ shown.length }} {{ shown.length === 1 ? 'match' : 'matches' }}</template>
        <template v-else>{{ shown.length }} {{ shown.length === 1 ? 'item' : 'items' }}</template>
      </p>
      <div class="border border-zinc-800 rounded-lg overflow-hidden">
        <div class="max-h-[50vh] overflow-y-auto">
          <p v-if="shown.length === 0" class="px-3 py-4 text-center text-xs text-zinc-500">
            {{ searching ? 'No matches' : 'Empty folder' }}
          </p>
          <component
            :is="item.is_dir || searching ? 'button' : 'div'"
            v-for="item in shown"
            :key="item.path"
            :type="item.is_dir || searching ? 'button' : undefined"
            :title="item.path"
            :class="[
              'w-full flex items-center gap-2 px-3 py-2 text-left text-sm border-b border-zinc-800/50 last:border-0',
              { 'hover:bg-zinc-800/60 transition-colors': item.is_dir || searching },
            ]"
            data-archive-item
            @click="openItem(item)"
          >
            <AppIcon
              :name="item.is_dir ? 'folder' : 'document'"
              :class="['size-4 shrink-0', item.is_dir ? 'text-yellow-400' : 'text-zinc-500']"
              :stroke-width="1.5"
            />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-zinc-300">{{ item.name }}</span>
              <span v-if="searching" class="block truncate text-xs text-zinc-500">
                {{ resultLocation(item) ? `in ${resultLocation(item)}` : 'here' }}
              </span>
            </span>
            <span class="shrink-0 text-zinc-500 text-xs tabular-nums whitespace-nowrap">{{ formatSize(item.size) }}</span>
          </component>
        </div>
      </div>

      <!-- Extract result -->
      <div
        v-if="extractSuccess"
        class="mt-3 text-sm text-green-400 bg-green-950/50 border border-green-900 rounded-lg px-3 py-2"
      >
        Extracted successfully!
      </div>
      <div
        v-if="extractError"
        class="mt-3 text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2"
      >
        {{ extractError }}
      </div>
    </div>

    <template #footer>
      <div class="flex flex-col gap-2">
        <!-- Optional extraction password (always shown when contents loaded) -->
        <div v-if="!loading && !passwordRequired && !error" class="flex gap-2">
          <SecretInput
            v-model="extractPassword"
            placeholder="Password (optional)"
            class="flex-1 bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-zinc-600 transition-colors"
            @keydown.enter="extract"
          />
        </div>
        <div class="flex justify-end gap-2">
          <button
            class="text-sm text-zinc-400 hover:text-white px-4 py-2 rounded-lg hover:bg-zinc-800 transition-colors"
            @click="close"
          >
            Close
          </button>
          <button
            :disabled="extracting || loading || !!error"
            class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white px-4 py-2 rounded-lg transition-colors"
            @click="extract"
          >
            <AppSpinner v-if="extracting" size="sm" />
            {{ extracting ? 'Extracting…' : 'Extract here' }}
          </button>
        </div>
      </div>
    </template>
  </AppModal>
</template>
