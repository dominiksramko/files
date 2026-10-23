// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, ref } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import { useCardInUrl } from '@/composables/useCardInUrl'
import { useToast } from '@/composables/useToast'
import { CARD_OPENED_STATE } from '@/utils/layerHistory'
import { parseKanban } from '@/utils/kanban'

const cleanups: (() => void)[] = []
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.useRealTimers()
})

// Lets router navigations, history traversal (Back/Forward) and watchers finish.
async function settle() {
  for (let round = 0; round < 5; round++) await new Promise((resolve) => setTimeout(resolve, 0))
}

// A board with two cards called Fix, shown at url.
async function mountBoard(url: string, { active = true, loaded = true } = {}) {
  // jsdom keeps one history across tests, and a router's first navigation
  // replaces the entry it starts on: start each test on a clean one.
  window.history.replaceState(null, '', url)
  const router = createRouter({ history: createWebHistory(), routes: [{ path: '/', component: { render: () => null } }] })
  await router.push(url)
  const board = ref(parseKanban(['## To do', '- Fix', '- Write docs', '', '## Done', '- Fix', ''].join('\n')))
  const openedCardId = ref<number | null>(null)
  const isActive = ref(active)
  const isLoaded = ref(loaded)
  const app = createApp(defineComponent({
    setup() {
      useCardInUrl({ board, openedCardId, active: isActive, loaded: isLoaded })
      return () => null
    },
  }))
  app.use(router)
  app.mount(document.createElement('div'))
  cleanups.push(() => app.unmount())
  await settle()

  const [firstFix, docs] = board.value.columns[0]!.cards
  const secondFix = board.value.columns[1]!.cards[0]!
  const query = () => router.currentRoute.value.query
  return { router, board, openedCardId, isLoaded, firstFix: firstFix!, docs: docs!, secondFix, query }
}

describe('useCardInUrl', () => {
  it('opens the card a link names once the board has been read', async () => {
    const { openedCardId, isLoaded, secondFix } = await mountBoard('/?preview=a.kanban&card=Fix&card_n=2', { loaded: false })
    expect(openedCardId.value).toBeNull()

    isLoaded.value = true
    await settle()
    expect(openedCardId.value).toBe(secondFix.id)
  })

  it('names the card it opens in the URL, in a history entry of its own', async () => {
    const { router, openedCardId, secondFix, query } = await mountBoard('/?preview=a.kanban')
    const entries = window.history.length

    openedCardId.value = secondFix.id
    await settle()
    expect(query()).toEqual({ preview: 'a.kanban', card: 'Fix', card_n: '2' })
    expect(window.history.length).toBe(entries + 1)
    expect(router.options.history.state[CARD_OPENED_STATE]).toBe(true)
  })

  it('goes back to the board when closing a card it opened', async () => {
    const { router, openedCardId, docs, query } = await mountBoard('/?preview=a.kanban')
    openedCardId.value = docs.id
    await settle()

    openedCardId.value = null
    await settle()
    expect(query()).toEqual({ preview: 'a.kanban' })
    expect(router.options.history.state[CARD_OPENED_STATE]).toBeUndefined()
  })

  it('closes the card on Back and opens it again on Forward', async () => {
    const { router, openedCardId, docs } = await mountBoard('/?preview=a.kanban')
    openedCardId.value = docs.id
    await settle()

    router.back()
    await settle()
    expect(openedCardId.value).toBeNull()

    router.forward()
    await settle()
    expect(openedCardId.value).toBe(docs.id)
  })

  it('takes a card opened from a link out of the URL in place when closing (there may be nothing to go back to)', async () => {
    const { openedCardId, query } = await mountBoard('/?preview=a.kanban&card=Write+docs')
    const entries = window.history.length

    openedCardId.value = null
    await settle()
    expect(query()).toEqual({ preview: 'a.kanban' })
    expect(window.history.length).toBe(entries)
  })

  it('says so and drops a card the board no longer has from the URL, opening none', async () => {
    const { openedCardId, query } = await mountBoard('/?preview=a.kanban&card=Renamed+since')
    expect(openedCardId.value).toBeNull()
    expect(query()).toEqual({ preview: 'a.kanban' })
    expect(useToast().toast.value?.message).toBe('That card is no longer on this board')
  })

  it('follows a rename into the URL once typing pauses, without new history entries', async () => {
    const { openedCardId, firstFix, query } = await mountBoard('/?preview=a.kanban&card=Fix')
    expect(openedCardId.value).toBe(firstFix.id)
    const entries = window.history.length
    vi.useFakeTimers()

    firstFix.title = 'Fix l'
    await vi.advanceTimersByTimeAsync(100)
    firstFix.title = 'Fix login'
    await vi.advanceTimersByTimeAsync(300)
    expect(query().card).toBe('Fix')

    await vi.advanceTimersByTimeAsync(100)
    expect(query()).toEqual({ preview: 'a.kanban', card: 'Fix login' })
    expect(window.history.length).toBe(entries)
    expect(openedCardId.value).toBe(firstFix.id)
  })

  it('renumbers the URL when the open card moves past another of the same title', async () => {
    const { board, openedCardId, secondFix, query } = await mountBoard('/?preview=a.kanban')
    openedCardId.value = secondFix.id
    await settle()
    vi.useFakeTimers()

    const [toDo, done] = board.value.columns
    done!.cards = []
    toDo!.cards.unshift(secondFix)
    await vi.advanceTimersByTimeAsync(400)
    expect(query()).toEqual({ preview: 'a.kanban', card: 'Fix' })
    expect(openedCardId.value).toBe(secondFix.id)
  })

  it('drops the card from the URL when it is deleted while open', async () => {
    const { board, openedCardId, docs, query } = await mountBoard('/?preview=a.kanban')
    openedCardId.value = docs.id
    await settle()

    board.value.columns[0]!.cards = board.value.columns[0]!.cards.filter((card) => card !== docs)
    await settle()
    expect(query()).toEqual({ preview: 'a.kanban' })
  })

  it('leaves the URL alone on a board that is not on show', async () => {
    const { openedCardId, docs, query } = await mountBoard('/?preview=b.kanban&card=Write+docs', { active: false })
    expect(openedCardId.value).toBeNull()

    openedCardId.value = docs.id
    await settle()
    expect(query()).toEqual({ preview: 'b.kanban', card: 'Write docs' })
  })
})
