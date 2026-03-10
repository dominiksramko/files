<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Plyr from 'plyr'
import 'plyr/dist/plyr.css'
import type { FileEntry } from '@/types'
import { files as filesApi, progress as progressApi } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{ entry: FileEntry }>()

// Derived once at mount; parent remounts this component (via :key) on path change.
const needsRemux = ['video/x-matroska', 'video/x-msvideo', 'video/quicktime'].includes(props.entry.mime)
const src        = needsRemux ? undefined : filesApi.streamUrl(props.entry.path, props.entry.mod_time)
const sourceType = needsRemux ? undefined : props.entry.mime
// Computed so it reacts if the store entry gets its thumb_hash populated by the
// SSE stream after the initial listing (thumbnail generated while folder is open).
const poster = computed(() =>
  props.entry.has_thumb && props.entry.thumb_hash
    ? filesApi.thumbUrl(props.entry.path, props.entry.thumb_hash)
    : undefined,
)

interface SubtitleTrack {
  src: string
  label: string
  srclang: string
}

const videoEl        = ref<HTMLVideoElement | null>(null)
const subtitles      = ref<SubtitleTrack[]>([])
const plyrControlsEl = ref<Element | null>(null)
let plyrInstance: Plyr | null = null
let hlsInstance: any = null
let alive = true

const { show: showToast } = useToast()

// --- Screenshot ---
const takingScreenshot = ref(false)

async function takeScreenshot() {
  if (!videoEl.value || takingScreenshot.value) return
  const timestamp = Math.floor(videoEl.value.currentTime)
  takingScreenshot.value = true
  try {
    await filesApi.screenshot(props.entry.path, timestamp)
    showToast('Screenshot taken', 'success')
  } catch {
    showToast('Screenshot failed', 'error')
  } finally {
    takingScreenshot.value = false
  }
}

// --- Resume prompt ---
const showResumePrompt = ref(false)
const savedPosition    = ref(0)

function formatTime(s: number): string {
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = Math.floor(s % 60)
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(ss).padStart(2, '0')}`
  return `${m}:${String(ss).padStart(2, '0')}`
}

function dismissResume() {
  showResumePrompt.value = false
}

function resumePlayback() {
  showResumePrompt.value = false
  if (plyrInstance) {
    plyrInstance.currentTime = savedPosition.value
    plyrInstance.play()
  }
}

// --- Progress saving ---
const FINISHED_THRESHOLD = 0.92
let progressInterval: ReturnType<typeof setInterval> | null = null
let seekTimer: ReturnType<typeof setTimeout> | null = null

async function saveOrClear() {
  if (!plyrInstance) return
  const pos = plyrInstance.currentTime
  const dur = plyrInstance.duration
  if (!dur || pos < 2) return
  if (pos / dur >= FINISHED_THRESHOLD) {
    await progressApi.delete(props.entry.path).catch(() => {})
    if (progressInterval) { clearInterval(progressInterval); progressInterval = null }
    return
  }
  progressApi.save(props.entry.path, pos, dur).catch(() => {})
}

function setupPlyr(el: HTMLVideoElement) {
  const plyrOptions: Plyr.Options = {
    controls: [
      'play-large', 'play', 'rewind', 'fast-forward',
      'progress', 'current-time', 'duration',
      'mute', 'volume', 'captions', 'settings', 'pip', 'fullscreen',
    ],
    settings: ['captions', 'speed'],
    captions: { active: true, language: 'auto', update: true },
    speed: { selected: 1, options: [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] },
    keyboard: { focused: true, global: false },
  }

  plyrInstance = new Plyr(el, plyrOptions)

  // Insert a placeholder span before the captions button and teleport the
  // screenshot button into it, so it appears in the native Plyr control bar.
  const captionsBtn = el.closest('.plyr')?.querySelector('[data-plyr="captions"]')
  if (captionsBtn?.parentElement) {
    const screenshotSlot = document.createElement('span')
    captionsBtn.parentElement.insertBefore(screenshotSlot, captionsBtn)
    plyrControlsEl.value = screenshotSlot
  }

  // Fetch saved progress once the player is ready
  plyrInstance.on('ready', async () => {
    if (!alive) return
    try {
      const prog = await progressApi.get(props.entry.path)
      if (!alive || !prog) return
      if (prog.duration && prog.position / prog.duration < FINISHED_THRESHOLD) {
        savedPosition.value = prog.position
        showResumePrompt.value = true
      }
    } catch {
      // no saved progress — nothing to do
    }
  })

  // Save on pause
  plyrInstance.on('pause', () => saveOrClear())

  // Save on seek (debounced)
  plyrInstance.on('seeked', () => {
    if (seekTimer) clearTimeout(seekTimer)
    seekTimer = setTimeout(() => saveOrClear(), 1000)
  })

  // Save every 30s while playing
  progressInterval = setInterval(() => {
    if (plyrInstance?.playing) saveOrClear()
  }, 30000)
}

const plyrOptions: Plyr.Options = {
  controls: [
    'play-large', 'play', 'rewind', 'fast-forward',
    'progress', 'current-time', 'duration',
    'mute', 'volume', 'captions', 'settings', 'pip', 'fullscreen',
  ],
  settings: ['captions', 'speed'],
  captions: { active: true, language: 'auto', update: true },
  speed: { selected: 1, options: [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] },
  keyboard: { focused: true, global: false },
}

onMounted(async () => {
  // Ask the server for all subtitle tracks (file-based + embedded).
  // This is the only network call the client makes — the server does all discovery.
  try {
    const raw = await filesApi.subtitles(props.entry.path)
    if (!alive) return
    subtitles.value = raw.map(t => ({
      src:     filesApi.subtitleUrl(t.url),
      label:   t.label,
      srclang: t.srclang,
    }))
  } catch {
    // subtitle discovery failed — continue without captions
  }

  // Wait for Vue to render <track> elements into the DOM before Plyr reads them.
  await nextTick()
  if (!alive || !videoEl.value) return

  if (needsRemux) {
    const Hls = (await import('hls.js')).default
    if (Hls.isSupported()) {
      hlsInstance = new Hls()
      hlsInstance.loadSource(filesApi.hlsPlaylistUrl(props.entry.path))
      hlsInstance.attachMedia(videoEl.value)

      hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
        if (alive && videoEl.value && !plyrInstance) {
          setupPlyr(videoEl.value!)
        }
      })
    } else if (videoEl.value.canPlayType('application/vnd.apple.mpegurl')) {
      videoEl.value.src = filesApi.hlsPlaylistUrl(props.entry.path)
      setupPlyr(videoEl.value)
    }
  } else {
    setupPlyr(videoEl.value)
  }
})

// Destroy Plyr before the DOM element is removed, so Plyr can clean up its
// own DOM manipulation before Vue unmounts the component.
onBeforeUnmount(() => {
  alive = false
  if (seekTimer) clearTimeout(seekTimer)
  if (progressInterval) { clearInterval(progressInterval); progressInterval = null }

  // Fire-and-forget final position save
  if (plyrInstance) {
    const pos = plyrInstance.currentTime
    const dur = plyrInstance.duration
    if (dur && pos >= 2 && pos / dur < FINISHED_THRESHOLD) {
      progressApi.save(props.entry.path, pos, dur).catch(() => {})
    }
  }

  if (hlsInstance) {
    hlsInstance.destroy()
    hlsInstance = null
  }
  plyrInstance?.destroy()
  plyrInstance = null
})
</script>

<template>
  <div class="plyr-wrapper w-full max-w-5xl relative" @click.stop>
    <video ref="videoEl" preload="metadata" playsinline :poster="poster">
      <source v-if="src" :src="src" :type="sourceType" />
      <track
        v-for="(sub, i) in subtitles"
        :key="sub.src"
        kind="subtitles"
        :label="sub.label"
        :srclang="sub.srclang || undefined"
        :src="sub.src"
        :default="i === 0"
      />
    </video>

    <!-- Screenshot button — teleported into Plyr's control bar -->
    <Teleport v-if="plyrControlsEl" :to="plyrControlsEl">
      <button
        class="plyr__controls__item plyr__control"
        type="button"
        data-screenshot
        title="Take screenshot"
        :disabled="takingScreenshot"
        @click.stop="takeScreenshot"
      >
        <AppSpinner v-if="takingScreenshot" size="sm" />
        <AppIcon v-else name="camera" class="size-4" />
      </button>
    </Teleport>

    <!-- Resume prompt -->
    <Transition name="fade">
      <div
        v-if="showResumePrompt"
        class="absolute bottom-16 left-1/2 -translate-x-1/2 z-10 flex items-center gap-3 bg-black/85 backdrop-blur-sm rounded-xl px-4 py-2.5 text-sm whitespace-nowrap"
      >
        <span class="text-zinc-300">Continue from <span class="font-medium text-white">{{ formatTime(savedPosition) }}</span>?</span>
        <button
          class="text-zinc-400 hover:text-white transition-colors"
          @click="dismissResume"
        >Start over</button>
        <button
          class="bg-blue-600 hover:bg-blue-500 text-white px-3 py-1 rounded-lg transition-colors"
          @click="resumePlayback"
        >Resume</button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
:deep(.plyr) {
  --plyr-color-main: #3b82f6;
  --plyr-video-controls-background: linear-gradient(rgba(0,0,0,0), rgba(0,0,0,0.65));
  --plyr-font-family: inherit;
  border-radius: 0.5rem;
  overflow: hidden;
  max-height: calc(100vh - 120px);
  width: 100%;
}
:deep(.plyr video) {
  max-height: calc(100vh - 120px);
  object-fit: contain;
}
:deep([data-screenshot] svg) {
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
