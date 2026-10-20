<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { files as filesApi, PasswordRequiredError } from '@/utils/api'
import { formatSize } from '@/utils/formatters'
import { archiveTree, searchArchive, withArchiveState, type ArchiveItem, type ArchiveState, type ArchiveTree } from '@/utils/archiveTree'
import { nameOf, parentOf, relativeTo } from '@/utils/paths'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppLoading from '@/components/ui/AppLoading.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import PasswordDialog from '@/components/ui/PasswordDialog.vue'
import SecretInput from '@/components/ui/SecretInput.vue'
import { useArchiveExtract } from '@/composables/useArchiveExtract'
import { useToast } from '@/composables/useToast'

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
// The file names are encrypted too: nothing is listed without the password,
// which then extracts as well.
const password = ref('')
const passwordRequired = ref(false)
const wrongPassword = ref(false)
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

// Escape in the search clears it; with nothing to clear it closes the modal.
function clearSearch(event: KeyboardEvent) {
  if (!search.value) return
  event.preventDefault()
  search.value = ''
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
const { extractArchive, passwordPrompt } = useArchiveExtract()
const { show: showToast } = useToast()

async function load(pw = '') {
  loading.value = true
  error.value = ''
  try {
    tree.value = archiveTree(await filesApi.archiveContents(props.state.path, pw))
    passwordRequired.value = false
  } catch (e: unknown) {
    if (e instanceof PasswordRequiredError) {
      passwordRequired.value = true
      wrongPassword.value = pw !== ''
    } else {
      error.value = e instanceof Error ? e.message : 'Failed to read archive'
    }
  } finally {
    loading.value = false
  }
}

async function unlock() {
  if (!password.value) return
  await load(password.value)
}

// Asks for a password only when one is needed: before trying when the
// listing shows encrypted files, else when the server says so.
async function extract() {
  extracting.value = true
  extractError.value = ''
  try {
    const extracted = await extractArchive(props.state.path, { password: password.value || undefined, encrypted: tree.value.encrypted })
    if (extracted === null) return
    showToast('Extracted successfully', 'success')
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
  <!-- The same height in every folder, so moving between them doesn't make
       the modal jump; a password to type or an error needs only its own. -->
  <AppModal :title="name" size="lg" :fixed-height="!passwordRequired && !error" @close="close">
    <!-- The archive's name once: the breadcrumb's first crumb is the title -->
    <template #title>
      <div class="min-w-0 flex-1 -my-2 -ml-2 flex">
        <FileBreadcrumb :path="folder" :root-label="name" root-icon="archive" :href-for="crumbHref" @navigate="openFolder" />
      </div>
    </template>

    <!-- Password prompt -->
    <div v-if="passwordRequired" class="space-y-3">
      <p class="flex items-center gap-1.5 text-sm text-zinc-400">
        <AppIcon name="lock" class="size-4 shrink-0" />
        Its file names are password protected.
      </p>
      <div class="flex gap-2">
        <SecretInput
          v-model="password"
          placeholder="Enter password"
          class="flex-1 bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
          @keydown.enter="unlock"
        />
        <AppButton variant="primary" :disabled="!password || loading" @click="unlock">
          <span class="flex items-center gap-1.5"><AppSpinner v-if="loading" size="sm" />Unlock</span>
        </AppButton>
      </div>
      <p v-if="wrongPassword" class="text-xs text-red-400">Wrong password</p>
    </div>

    <!-- Loading -->
    <AppLoading v-else-if="loading" label="Reading the archive…" class="h-full">
      <p class="text-xs text-zinc-500">Large archives can take a few seconds.</p>
    </AppLoading>

    <!-- Error -->
    <div v-else-if="error" class="text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2">
      {{ error }}
    </div>

    <!-- Contents: the list takes the height left and scrolls -->
    <div v-else class="h-full flex flex-col">
      <!-- Search -->
      <div class="shrink-0 relative mb-2">
        <AppIcon name="search" class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none" />
        <input
          v-model="search"
          type="text"
          autocomplete="off"
          :placeholder="folder ? 'Search this folder…' : 'Search the archive…'"
          class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg pl-8 pr-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
          @keydown.escape="clearSearch"
        />
      </div>
      <p class="shrink-0 text-xs text-zinc-500 mb-2">
        <template v-if="searching">{{ shown.length }} {{ shown.length === 1 ? 'match' : 'matches' }}</template>
        <template v-else>{{ shown.length }} {{ shown.length === 1 ? 'item' : 'items' }}</template>
        <span v-if="tree.encrypted" class="inline-flex items-center gap-1 ml-1" data-archive-encrypted>
          · <AppIcon name="lock" class="size-3" /> extracting needs a password
        </span>
      </p>
      <div class="flex-1 min-h-0 flex flex-col border border-zinc-800 rounded-lg overflow-hidden">
        <div class="flex-1 min-h-0 overflow-y-auto" data-archive-list>
          <p v-if="shown.length === 0" class="px-3 py-4 text-center text-xs text-zinc-500">
            {{ searching ? 'No matches' : 'Empty folder' }}
          </p>
          <component
            :is="item.is_dir || searching ? 'button' : 'div'"
            v-for="item in shown"
            :key="item.path"
            :type="item.is_dir || searching ? 'button' : undefined"
            :title="item.encrypted ? `${item.path} (password protected)` : item.path"
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
            <AppIcon v-if="item.encrypted" name="lock" class="size-3.5 shrink-0 text-zinc-500" data-archive-lock />
            <span class="shrink-0 text-zinc-500 text-xs tabular-nums whitespace-nowrap">{{ formatSize(item.size) }}</span>
          </component>
        </div>
      </div>

      <!-- Extract result -->
      <div
        v-if="extractError"
        class="shrink-0 mt-3 text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2"
      >
        {{ extractError }}
      </div>
    </div>

    <!-- Inside the modal, so it's the dialog on top -->
    <PasswordDialog :prompt="passwordPrompt" action="Extract" busy-label="Extracting…" />

    <template #footer>
      <div class="flex justify-end gap-2">
        <AppButton @click="close">Close</AppButton>
        <AppButton variant="primary" :disabled="extracting || loading || passwordRequired || !!error" @click="extract">
          <span class="flex items-center gap-1.5">
            <AppSpinner v-if="extracting" size="sm" />
            {{ extracting ? 'Extracting…' : 'Extract here' }}
          </span>
        </AppButton>
      </div>
    </template>
  </AppModal>
</template>
