// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import type { UsageEntry } from '@/types'
import UsageList from '@/components/usage/UsageList.vue'

const cleanups: (() => void)[] = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

const folder: UsageEntry = {
  name: 'videos', path: 'media/videos', is_dir: true, size: 0, mod_time: '2026-10-20T10:00:00Z', mime: '',
  has_thumb: false, files: 1, dirs: 0, disk: 4096, scanned: true,
}

// Mounts the list with one folder; returns its row, the selection and the
// folders opened.
function mountList() {
  const selected = ref<string[]>([])
  const opened: string[] = []
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () => h(UsageList, {
      entries: [folder],
      total: 4096,
      root: 'media',
      selected: selected.value,
      'onUpdate:selected': (paths: string[]) => { selected.value = paths },
      onOpen: (entry: UsageEntry) => opened.push(entry.path),
    }),
  })
  app.mount(root)
  cleanups.push(() => { app.unmount(); root.remove() })
  return { row: root.querySelector('li')!, selected, opened }
}

describe('UsageList', () => {
  it('selects from anywhere in the strip around the checkbox, not just the box', async () => {
    const { row, selected, opened } = mountList()
    const strip = row.firstElementChild as HTMLElement
    strip.click()
    expect(selected.value).toEqual(['media/videos'])
    await nextTick()
    strip.querySelector('button')!.click()
    expect(selected.value).toEqual([])
    expect(opened).toEqual([])
  })

  it('opens a folder from the rest of the row', () => {
    const { row, selected, opened } = mountList()
    ;(row.querySelector('.truncate') as HTMLElement).click()
    expect(opened).toEqual(['media/videos'])
    expect(selected.value).toEqual([])
  })
})
