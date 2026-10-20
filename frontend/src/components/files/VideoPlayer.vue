<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { debounce } from '@/utils/debounce'
import Plyr from 'plyr'
import 'plyr/dist/plyr.css'
import type { FileEntry } from '@/types'
import { files as filesApi, progress as progressApi } from '@/utils/api'
import { formatDuration } from '@/utils/formatters'
import { useToast } from '@/composables/useToast'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{ entry: FileEntry; active?: boolean }>()

// Derived once at mount; parent remounts this component (via :key) on path change.
// Only MP4 and WebM play natively everywhere; every other container goes through server-side HLS.
const needsRemux = !['video/mp4', 'video/webm'].includes(props.entry.mime)
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

const videoEl            = ref<HTMLVideoElement | null>(null)
const subtitles          = ref<SubtitleTrack[]>([])
const plyrControlsEl = ref<Element | null>(null) // screenshot slot
let plyrInstance: Plyr | null = null
let hlsInstance: any = null
let plyrEl: Element | null = null
let alive = true
let controlsHideTimer: ReturnType<typeof setTimeout> | null = null
let controlsVisible = true

// Check live at call time so CSS layout updates on resize without reinitialising Plyr.
const isMobileNow = () => window.matchMedia('(max-width: 480px)').matches

function showControls() {
  controlsVisible = true
  plyrEl?.classList.remove('plyr--hide-controls')
  scheduleControlsHide()
}

function scheduleControlsHide() {
  if (controlsHideTimer) clearTimeout(controlsHideTimer)
  // Only hide while playing; stay visible when paused.
  controlsHideTimer = setTimeout(() => {
    if (plyrInstance?.playing) {
      controlsVisible = false
      plyrEl?.classList.add('plyr--hide-controls')
    }
  }, 5000)
}

const { show: showToast } = useToast()

// Streaming through the server starts a transcode there: a video in a panel
// next to the open file waits until it's opened itself.
let waitingUntilActive = false

watch(() => props.active, (isActive) => {
  if (isActive && waitingUntilActive) {
    waitingUntilActive = false
    void attachSource()
    return
  }
  if (!plyrInstance) return
  if (isActive) plyrInstance.play()?.catch(() => {})
  else plyrInstance.pause()
})

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
const debouncedSaveOrClear = debounce(saveOrClear, 1000)

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
    // We manage auto-hide and click-to-play ourselves.
    hideControls: false,
    clickToPlay: false,
  }

  plyrInstance = new Plyr(el, plyrOptions)
  plyrEl = el.closest('.plyr')

  // Inject screenshot slot before captions in the control bar.
  const captionsBtn = plyrEl?.querySelector('[data-plyr="captions"]')
  if (captionsBtn?.parentElement) {
    const screenshotSlot = document.createElement('span')
    screenshotSlot.className = 'plyr__screenshot-slot'
    captionsBtn.parentElement.insertBefore(screenshotSlot, captionsBtn)
    plyrControlsEl.value = screenshotSlot
  }

  // Click/tap video: if controls are hidden, first interaction shows them only.
  // If controls are visible, it toggles playback.
  const videoWrapper = plyrEl?.querySelector('.plyr__video-wrapper')
  let controlsVisibleAtTouchStart = true
  videoWrapper?.addEventListener('touchstart', () => {
    controlsVisibleAtTouchStart = controlsVisible
  }, { passive: true })
  videoWrapper?.addEventListener('touchend', (event) => {
    event.preventDefault()
    showControls()
    if (!controlsVisibleAtTouchStart || !plyrInstance) return
    if (plyrInstance.playing) plyrInstance.pause()
    else plyrInstance.play()?.catch(() => {})
  })

  // Desktop click on video area (touch fires touchend which calls preventDefault,
  // so this only runs on genuine mouse clicks).
  videoWrapper?.addEventListener('click', () => {
    if (!controlsVisible) { showControls(); return }
    if (!plyrInstance) return
    if (plyrInstance.playing) plyrInstance.pause()
    else plyrInstance.play()?.catch(() => {})
  })

  // Mouse movement anywhere in the player shows controls and resets the 5s timer.
  plyrEl?.addEventListener('mousemove', showControls)

  // Touching the control bar resets the auto-hide timer.
  plyrEl?.querySelector('.plyr__controls')?.addEventListener('touchstart', showControls, { passive: true })

  // Auto-hide after 5s while playing; keep visible when paused.
  plyrInstance.on('play', () => scheduleControlsHide())
  plyrInstance.on('pause', () => {
    if (controlsHideTimer) { clearTimeout(controlsHideTimer); controlsHideTimer = null }
    controlsVisible = true
    plyrEl?.classList.remove('plyr--hide-controls')
  })

  // Fetch saved progress once the player is ready.
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
  plyrInstance.on('seeked', () => debouncedSaveOrClear())

  // Save every 30s while playing
  progressInterval = setInterval(() => {
    if (plyrInstance?.playing) saveOrClear()
  }, 30000)
}

onMounted(async () => {
  // Ask the server for all subtitle tracks (file-based + embedded).
  // This is the only network call the client makes — the server does all discovery.
  try {
    const raw = await filesApi.subtitles(props.entry.path)
    if (!alive) return
    subtitles.value = raw.map(t => ({
      src:     t.url,
      label:   t.label,
      srclang: t.srclang,
    }))
  } catch {
    // subtitle discovery failed — continue without captions
  }

  // Wait for Vue to render <track> elements into the DOM before Plyr reads them.
  await nextTick()
  if (!alive || !videoEl.value) return

  if (needsRemux && !props.active) waitingUntilActive = true
  else await attachSource()
})

async function attachSource() {
  if (!alive || !videoEl.value) return
  if (needsRemux) {
    const Hls = (await import('hls.js')).default
    // The panel may have closed while the library loaded.
    if (!alive || !videoEl.value) return
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
}

// Destroy Plyr before the DOM element is removed, so Plyr can clean up its
// own DOM manipulation before Vue unmounts the component.
onBeforeUnmount(() => {
  alive = false
  debouncedSaveOrClear.cancel()
  if (progressInterval)  { clearInterval(progressInterval);  progressInterval  = null }
  if (controlsHideTimer) { clearTimeout(controlsHideTimer);  controlsHideTimer = null }

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
  <div class="plyr-wrapper w-full relative" @click.stop @touchstart.stop @touchmove.stop>
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
        <span class="text-zinc-300">Continue from <span class="font-medium text-white">{{ formatDuration(savedPosition) }}</span>?</span>
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
  max-height: calc(100vh - 80px);
  width: 100%;
}
:deep(.plyr video) {
  max-height: calc(100vh - 80px);
  object-fit: contain;
}
:deep([data-screenshot] svg) {
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

/* ── Two-row control bar (all screen sizes) ───────────────────────────────── */
:deep(.plyr__controls) {
  flex-wrap: wrap;
  align-items: center;
}
/* Progress bar: full-width first row with breathing room above buttons */
:deep(.plyr__progress__container) {
  order: -10;
  flex: 0 0 100%;
  margin-bottom: 6px;
}

/* Remove Plyr's default gap after the play button (all screen sizes) */
:deep([data-plyr="play"]) {
  margin-right: 0 !important;
}

/* Desktop: group left controls tightly, push screenshot + right-side controls to the far right */
@media (min-width: 481px) {
  :deep([data-plyr="rewind"]) {
    margin-left: 0 !important;
  }
  :deep(.plyr__screenshot-slot) {
    margin-left: auto;
  }
}

/* ── Mobile-only adjustments (≤480px) ────────────────────────────────────── */
@media (max-width: 480px) {
  /* Pull play, mute, and duration to the front */
  :deep([data-plyr="play"])      { order: -4; }
  :deep(.plyr__volume)           { order: -3; }
  :deep(.plyr__time--current)    { order: -2; }
  :deep(.plyr__time--duration)   { order: -1; }

  /* Hide desktop-only controls */
  :deep([data-plyr="rewind"]),
  :deep([data-plyr="fast-forward"]) {
    display: none;
  }
  /* Hide only the volume slider, keep the mute button visible */
  :deep(.plyr__volume input[type="range"]) {
    display: none;
  }
  /* Push screenshot + right-side controls to the far right on mobile */
  :deep(.plyr__screenshot-slot) {
    margin-left: auto;
  }
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
