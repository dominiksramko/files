<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import type { SizeMeasure, UsageEntry } from '@/types'
import { canOpen, formatShare, groupSmallEntries, shareOf, sizeOf, squarify, type SmallEntries } from '@/utils/diskUsage'
import { catBg, catIconColor, catIconName, entryCategory, formatSize } from '@/utils/formatters'
import AppIcon from '@/components/ui/AppIcon.vue'

// The folder's items as rectangles sized by what they take. Items too small
// to see are added up into one tile.
const props = defineProps<{ entries: UsageEntry[]; measure: SizeMeasure; total: number; selected: string[] }>()
const emit = defineEmits<{ open: [entry: UsageEntry]; select: [entry: UsageEntry] }>()

// Below this part of the total an item joins the "smaller items" tile.
const MIN_TILE_SHARE = 0.004

const container = ref<HTMLElement | null>(null)
const width = ref(0)
const height = ref(0)

let resizeObserver: ResizeObserver | null = null
watch(container, (element) => {
  resizeObserver?.disconnect()
  if (!element) return
  resizeObserver = new ResizeObserver(() => {
    width.value = element.clientWidth
    height.value = element.clientHeight
  })
  resizeObserver.observe(element)
})
onUnmounted(() => resizeObserver?.disconnect())

type Tile = { entry: UsageEntry; rest?: undefined } | { entry?: undefined; rest: SmallEntries }

const tiles = computed(() => {
  const { shown, rest } = groupSmallEntries(props.entries, props.measure, props.total, MIN_TILE_SHARE)
  const items: { item: Tile; size: number }[] = shown.map((entry) => ({ item: { entry }, size: sizeOf(entry, props.measure) }))
  if (rest) items.push({ item: { rest }, size: rest.size })
  return squarify(items, width.value, height.value)
})

function tileClass(tile: Tile): string[] {
  if (!tile.entry) return ['bg-zinc-800/60 text-zinc-500']
  const category = entryCategory(tile.entry.mime, tile.entry.name, tile.entry.is_dir)
  return [
    catBg[category],
    'cursor-pointer hover:brightness-150',
    props.selected.includes(tile.entry.path) ? 'ring-2 ring-blue-400 ring-inset' : '',
  ]
}

function tileTitle(tile: Tile): string {
  const size = tile.entry ? sizeOf(tile.entry, props.measure) : tile.rest.size
  const name = tile.entry ? tile.entry.name : `${tile.rest.count.toLocaleString()} smaller items`
  return `${name}: ${formatSize(size)} (${formatShare(shareOf(size, props.total))})`
}

function onTileClick(tile: Tile) {
  if (!tile.entry) return
  if (canOpen(tile.entry)) emit('open', tile.entry)
  else emit('select', tile.entry)
}
</script>

<template>
  <div ref="container" class="relative overflow-hidden rounded-lg bg-zinc-900 border border-zinc-800">
    <div
      v-for="tile in tiles"
      :key="tile.item.entry?.path ?? '(smaller items)'"
      class="absolute p-px"
      :style="{ left: `${tile.x}px`, top: `${tile.y}px`, width: `${tile.width}px`, height: `${tile.height}px` }"
    >
      <div
        :class="['size-full rounded-sm overflow-hidden flex flex-col justify-start gap-0.5 p-1.5 transition-[filter]', ...tileClass(tile.item)]"
        :title="tileTitle(tile.item)"
        @click="onTileClick(tile.item)"
      >
        <template v-if="tile.width > 64 && tile.height > 34">
          <div class="flex items-center gap-1 min-w-0">
            <AppIcon
              v-if="tile.item.entry"
              :name="catIconName[entryCategory(tile.item.entry.mime, tile.item.entry.name, tile.item.entry.is_dir)]"
              :class="['size-3.5 shrink-0', catIconColor[entryCategory(tile.item.entry.mime, tile.item.entry.name, tile.item.entry.is_dir)]]"
            />
            <span class="truncate text-xs text-zinc-100">
              {{ tile.item.entry ? tile.item.entry.name : `${tile.item.rest.count.toLocaleString()} smaller items` }}
            </span>
          </div>
          <span class="text-[11px] text-zinc-400 tabular-nums">
            {{ formatSize(tile.item.entry ? sizeOf(tile.item.entry, measure) : tile.item.rest.size) }}
          </span>
        </template>
      </div>
    </div>
  </div>
</template>
