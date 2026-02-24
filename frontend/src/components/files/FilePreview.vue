<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'

const props = defineProps<{
  entry: FileEntry | null
  hasPrev: boolean
  hasNext: boolean
  index: number
  total: number
}>()

const emit = defineEmits<{
  close: []
  prev: []
  next: []
}>()

const streamUrl = computed(() => (props.entry ? filesApi.streamUrl(props.entry.path) : ''))
const mime = computed(() => props.entry?.mime ?? '')
const isImage = computed(() => mime.value.startsWith('image/'))
const isVideo = computed(() => mime.value.startsWith('video/'))
const isAudio = computed(() => mime.value.startsWith('audio/'))
const isPdf = computed(() => mime.value === 'application/pdf')

function onKey(e: KeyboardEvent) {
  if (!props.entry) return
  if (e.key === 'Escape') emit('close')
  if (e.key === 'ArrowLeft' && props.hasPrev) emit('prev')
  if (e.key === 'ArrowRight' && props.hasNext) emit('next')
}

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <Transition name="lightbox">
      <div
        v-if="entry"
        class="fixed inset-0 z-50 flex flex-col bg-black/90 backdrop-blur-sm"
        @click.self="emit('close')"
      >
        <!-- Top bar -->
        <div class="flex items-center justify-between px-4 py-3 shrink-0">
          <span class="text-sm text-white font-medium truncate max-w-xs">{{ entry.name }}</span>
          <div class="flex items-center gap-3 shrink-0 ml-4">
            <!-- Counter -->
            <span class="text-xs text-zinc-400 tabular-nums">{{ index + 1 }} / {{ total }}</span>
            <!-- Download -->
            <a
              :href="filesApi.downloadUrl(entry.path)"
              target="_blank"
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              title="Download"
              @click.stop
            >
              <svg class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
            </a>
            <!-- Close -->
            <button
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              @click="emit('close')"
            >
              <svg class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Main content + side arrows -->
        <div class="flex-1 flex items-center min-h-0 gap-2 px-2">
          <!-- Prev arrow -->
          <button
            :class="[
              'shrink-0 p-2 rounded-full transition-all',
              hasPrev
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click="emit('prev')"
          >
            <svg class="size-7" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
            </svg>
          </button>

          <!-- Media content -->
          <div class="flex-1 flex items-center justify-center min-h-0 min-w-0">
            <!-- Image -->
            <img
              v-if="isImage"
              :key="entry.path"
              :src="streamUrl"
              :alt="entry.name"
              class="max-w-full max-h-full object-contain rounded-lg select-none"
              style="max-height: calc(100vh - 120px)"
              draggable="false"
            />

            <!-- Video -->
            <video
              v-else-if="isVideo"
              :key="entry.path"
              :src="streamUrl"
              controls
              class="max-w-full rounded-lg"
              style="max-height: calc(100vh - 120px)"
              preload="metadata"
            />

            <!-- Audio -->
            <div v-else-if="isAudio" class="w-full max-w-md px-6">
              <div class="flex items-center gap-5 mb-6">
                <div class="size-20 rounded-2xl bg-pink-950/60 flex items-center justify-center shrink-0">
                  <svg class="size-10 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                      d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
                  </svg>
                </div>
                <p class="text-white font-medium text-lg leading-tight">{{ entry.name }}</p>
              </div>
              <audio :key="entry.path" :src="streamUrl" controls class="w-full" />
            </div>

            <!-- PDF -->
            <iframe
              v-else-if="isPdf"
              :key="entry.path"
              :src="streamUrl"
              class="w-full rounded-lg border border-zinc-700"
              style="height: calc(100vh - 120px)"
              title="PDF preview"
            />
          </div>

          <!-- Next arrow -->
          <button
            :class="[
              'shrink-0 p-2 rounded-full transition-all',
              hasNext
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click="emit('next')"
          >
            <svg class="size-7" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>

        <!-- Bottom padding -->
        <div class="shrink-0 h-4" />
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.lightbox-enter-active,
.lightbox-leave-active {
  transition: opacity 0.15s ease;
}
.lightbox-enter-from,
.lightbox-leave-to {
  opacity: 0;
}
</style>
