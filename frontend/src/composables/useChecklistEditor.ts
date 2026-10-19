import { nextTick, type Ref } from 'vue'

interface ChecklistItem {
  id: number
  text: string
  done: boolean
}

// Keyboard-driven editing for a list of TodoRow checkboxes: Enter adds a row
// below, Backspace in an empty row deletes it and focuses the row above.
// Bind rows with `:ref="(instance) => setRowRef(item.id, instance)"`.
export function useChecklistEditor<T extends ChecklistItem>(items: Ref<T[]>, makeItem: () => T) {
  const rowRefs = new Map<number, { focus: () => void }>()

  function setRowRef(id: number, instance: unknown) {
    if (instance) rowRefs.set(id, instance as { focus: () => void })
    else rowRefs.delete(id)
  }

  async function focusItem(id: number) {
    await nextTick()
    rowRefs.get(id)?.focus()
  }

  function toggle(item: T) {
    item.done = !item.done
  }

  function removeItem(id: number) {
    const index = items.value.findIndex((item) => item.id === id)
    if (index !== -1) items.value.splice(index, 1)
  }

  function newItemAfter(item: T) {
    const index = items.value.findIndex((entry) => entry.id === item.id)
    const fresh = makeItem()
    items.value.splice(index + 1, 0, fresh)
    focusItem(fresh.id)
  }

  function addItem() {
    const fresh = makeItem()
    items.value.push(fresh)
    focusItem(fresh.id)
  }

  // visibleItems: the rows shown in the list the item belongs to, when that's
  // a filtered view of `items` (e.g. only the open tasks).
  function onBackspaceEmpty(item: T, visibleItems: T[] = items.value) {
    const visibleIndex = visibleItems.findIndex((entry) => entry.id === item.id)
    const previous = visibleIndex > 0 ? visibleItems[visibleIndex - 1] : null
    removeItem(item.id)
    if (previous) focusItem(previous.id)
  }

  return { setRowRef, focusItem, toggle, removeItem, newItemAfter, addItem, onBackspaceEmpty }
}
