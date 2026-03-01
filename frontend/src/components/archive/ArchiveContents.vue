<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import type { FileEntry, ArchiveEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { formatSize } from '@/utils/formatters'
import AppModal from '@/components/ui/AppModal.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{ entry: FileEntry }>()
const emit = defineEmits<{ close: []; extracted: [] }>()

const contents = ref<ArchiveEntry[]>([])
const loading = ref(true)
const error = ref('')
const password = ref('')
const passwordRequired = ref(false)
const search = ref('')

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return contents.value
  return contents.value.filter((item) => item.name.toLowerCase().includes(q))
})

const extracting = ref(false)
const extractError = ref('')
const extractSuccess = ref(false)

async function load(pw = '') {
  loading.value = true
  error.value = ''
  try {
    contents.value = await filesApi.archiveContents(props.entry.path, pw)
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
  try {
    await filesApi.extract(props.entry.path, undefined, password.value || undefined)
    extractSuccess.value = true
    emit('extracted')
  } catch (e: unknown) {
    extractError.value = e instanceof Error ? e.message : 'Extraction failed'
  } finally {
    extracting.value = false
  }
}

onMounted(() => load())
</script>

<template>
  <AppModal :title="entry.name" size="lg" @close="emit('close')">
    <!-- Password prompt -->
    <div v-if="passwordRequired" class="space-y-3">
      <p class="text-sm text-zinc-400">{{ error }}</p>
      <div class="flex gap-2">
        <input
          v-model="password"
          type="password"
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
      <!-- Search -->
      <div class="relative mb-2">
        <svg
          class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none"
          fill="none" viewBox="0 0 24 24" stroke="currentColor"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M21 21l-4.35-4.35M17 11A6 6 0 115 11a6 6 0 0112 0z" />
        </svg>
        <input
          v-model="search"
          type="text"
          placeholder="Filter files…"
          class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg pl-8 pr-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
          @keydown.escape="search = ''"
        />
      </div>
      <p class="text-xs text-zinc-500 mb-2">
        <template v-if="search.trim()">{{ filtered.length }} of {{ contents.length }} items</template>
        <template v-else>{{ contents.length }} items</template>
      </p>
      <div class="border border-zinc-800 rounded-lg overflow-hidden">
        <div class="max-h-80 overflow-y-auto">
          <table class="w-full text-sm">
            <tbody>
              <tr
                v-if="filtered.length === 0"
                class="border-b border-zinc-800/50 last:border-0"
              >
                <td colspan="2" class="px-3 py-4 text-center text-xs text-zinc-500">No matches</td>
              </tr>
              <tr
                v-for="item in filtered"
                :key="item.name"
                class="border-b border-zinc-800/50 last:border-0"
              >
                <td class="px-3 py-2">
                  <div class="flex items-center gap-2 min-w-0">
                    <svg
                      :class="['size-4 shrink-0', item.is_dir ? 'text-yellow-400' : 'text-zinc-500']"
                      fill="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        v-if="item.is_dir"
                        d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"
                      />
                      <path
                        v-else
                        d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
                        stroke="currentColor"
                        stroke-width="1.5"
                        fill="none"
                      />
                    </svg>
                    <span class="truncate text-zinc-300">{{ item.name }}</span>
                  </div>
                </td>
                <td class="px-3 py-2 text-right text-zinc-500 text-xs tabular-nums whitespace-nowrap">
                  {{ item.is_dir ? '' : formatSize(item.size) }}
                </td>
              </tr>
            </tbody>
          </table>
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
      <div class="flex justify-end gap-2">
        <button
          class="text-sm text-zinc-400 hover:text-white px-4 py-2 rounded-lg hover:bg-zinc-800 transition-colors"
          @click="emit('close')"
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
    </template>
  </AppModal>
</template>
