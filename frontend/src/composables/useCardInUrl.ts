import { computed, onUnmounted, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useToast } from '@/composables/useToast'
import { debounce } from '@/utils/debounce'
import { CARD_OPENED_STATE, layerHistoryStep } from '@/utils/layerHistory'
import {
  cardLocatorFrom,
  findCard,
  locateCard,
  withCardLocator,
  type CardLocator,
  type KanbanBoard,
} from '@/utils/kanban'

interface CardInUrlOptions {
  board: Ref<KanbanBoard>
  // The card whose editor is open; null when none is.
  openedCardId: Ref<number | null>
  // Whether this board is the preview being shown: only that one follows the URL.
  active: MaybeRefOrGetter<boolean>
  // Whether the board has been read; the URL's card waits for it.
  loaded: Ref<boolean>
}

const locatorKey = (locator: CardLocator | null) => (locator ? `${locator.occurrence}:${locator.title}` : null)

// Keeps a board's open card in the URL (?card=, see cardLocatorFrom), both ways:
// opening, renaming, moving or closing the card updates the URL, and a link, a
// refresh, Back or Forward opens or closes the card. History steps as for the
// preview (layerHistoryStep): opening adds an entry, so a phone's Back closes it.
export function useCardInUrl({ board, openedCardId, active, loaded }: CardInUrlOptions) {
  const route = useRoute()
  const router = useRouter()
  const { show: showToast } = useToast()

  const cardInUrl = computed(() => (toValue(active) ? cardLocatorFrom(route.query) : null))
  const openedLocator = computed(() =>
    openedCardId.value === null ? null : locateCard(board.value, openedCardId.value),
  )

  // Renaming writes the URL once typing pauses, not on every key (browsers
  // limit how often a page may rewrite it).
  const replaceAfterTyping = debounce((locator: CardLocator) => {
    router.replace({ query: withCardLocator(route.query, locator) })
  }, 400)

  // URL → editor. Watching the key, not the query, so a change to another key
  // doesn't look the card up again while a rename is still on its way to the URL.
  // A new card in the URL (Back, Forward) also drops a rename still waiting.
  watch(
    [() => locatorKey(cardInUrl.value), loaded],
    () => {
      replaceAfterTyping.cancel()
      if (!loaded.value || !toValue(active)) return
      const locator = cardInUrl.value
      if (!locator) {
        openedCardId.value = null
        return
      }
      const card = findCard(board.value, locator)
      if (card) {
        openedCardId.value = card.id
        return
      }
      showToast('That card is no longer on this board', 'info')
      router.replace({ query: withCardLocator(route.query, null) })
    },
    { immediate: true },
  )

  // Editor → URL.
  watch(
    () => locatorKey(openedLocator.value),
    (shownKey) => {
      if (!loaded.value || !toValue(active)) return
      replaceAfterTyping.cancel()
      const shown = openedLocator.value
      const step = layerHistoryStep(
        shownKey,
        locatorKey(cardInUrl.value),
        router.options.history.state[CARD_OPENED_STATE] === true,
      )
      if (step === 'push') router.push({ query: withCardLocator(route.query, shown), state: { [CARD_OPENED_STATE]: true } })
      else if (step === 'replace' && shown) replaceAfterTyping(shown)
      else if (step === 'replace') router.replace({ query: withCardLocator(route.query, null) })
      else if (step === 'back') router.back()
    },
  )

  onUnmounted(() => replaceAfterTyping.cancel())
}
