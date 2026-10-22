// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import { dragOverNearestList, nearestSpanIndex, sortableOf } from '@/utils/sortable'

const cleanups: (() => void)[] = []
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.restoreAllMocks()
})

describe('nearestSpanIndex', () => {
  const spans = [{ left: 0, right: 100 }, { left: 120, right: 220 }]

  it('picks the span containing x', () => {
    expect(nearestSpanIndex(spans, 50)).toBe(0)
    expect(nearestSpanIndex(spans, 120)).toBe(1)
  })

  it('picks the nearer span from the gap between them', () => {
    expect(nearestSpanIndex(spans, 105)).toBe(0)
    expect(nearestSpanIndex(spans, 115)).toBe(1)
  })

  it('picks the first or last span from beyond either end', () => {
    expect(nearestSpanIndex(spans, -40)).toBe(0)
    expect(nearestSpanIndex(spans, 900)).toBe(1)
  })

  it('returns -1 without spans', () => {
    expect(nearestSpanIndex([], 10)).toBe(-1)
  })
})

describe('sortableOf', () => {
  // Guards the private SortableJS hook dragOverNearestList relies on (see CLAUDE.md, Drag and drop).
  it('finds the instance vue-draggable-plus puts on its list, with its internal _onDragOver hook', async () => {
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => h(VueDraggable, { modelValue: ['a'], class: 'list' }, () => [h('div', 'a')]),
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    await nextTick()

    expect(sortableOf(root.querySelector('.list')!)).not.toBeNull()
  })

  it('returns null for an element that is not a list', () => {
    expect(sortableOf(document.createElement('div'))).toBeNull()
  })
})

describe('dragOverNearestList', () => {
  // Two lists side by side, 100px wide with a 20px gap, each holding one item.
  function makeLists() {
    return [0, 120].map((left) => {
      const list = document.createElement('div')
      const item = document.createElement('div')
      list.append(item)
      document.body.append(list)
      cleanups.push(() => list.remove())
      vi.spyOn(list, 'getBoundingClientRect').mockReturnValue(
        { left, right: left + 100, width: 100, top: 0, bottom: 50, height: 50 } as DOMRect,
      )
      const onDragOver = vi.fn()
      Object.assign(list, { Sortable1700000000000: { _onDragOver: onDragOver } })
      return { list, item, onDragOver }
    })
  }

  function pointAt(hitAt: (x: number, y: number) => Element | null) {
    Object.assign(document, { elementFromPoint: vi.fn(hitAt) })
    cleanups.push(() => Reflect.deleteProperty(document, 'elementFromPoint'))
  }

  it('hands a pointer below the lists to the nearest one, on its centre line', () => {
    const [first, second] = makeLists()
    pointAt(() => document.body)

    dragOverNearestList([first!.list, second!.list], 112, 400)

    expect(first!.onDragOver).not.toHaveBeenCalled()
    expect(second!.onDragOver).toHaveBeenCalledWith(
      { clientX: 170, clientY: 400, target: second!.list, rootEl: second!.list },
    )
  })

  it('targets the item on the centre line when there is one', () => {
    const [first, second] = makeLists()
    pointAt((x) => (x === 50 ? first!.item : document.body))

    dragOverNearestList([first!.list, second!.list], 104, 20)

    expect(first!.onDragOver).toHaveBeenCalledWith(
      { clientX: 50, clientY: 20, target: first!.item, rootEl: first!.list },
    )
  })

  it('leaves a pointer that is over a list to SortableJS', () => {
    const [first, second] = makeLists()
    pointAt(() => second!.item)

    dragOverNearestList([first!.list, second!.list], 150, 20)

    expect(first!.onDragOver).not.toHaveBeenCalled()
    expect(second!.onDragOver).not.toHaveBeenCalled()
  })

  it('does nothing without lists', () => {
    pointAt(() => document.body)
    expect(() => dragOverNearestList([], 10, 10)).not.toThrow()
  })
})
