import { describe, expect, it } from 'vitest'
import {
  adoptBoardIds,
  cardLocatorFrom,
  findCard,
  locateCard,
  parseKanban,
  withCardKept,
  withCardLocator,
  type KanbanBoard,
} from '@/utils/kanban'

const cardNamed = (board: KanbanBoard, title: string) =>
  board.columns.flatMap((column) => column.cards).find((card) => card.title === title)!

describe('adoptBoardIds', () => {
  const previousText = [
    '## To do',
    '- Design',
    '  - [ ] sketch',
    '  - [x] review',
    '- Build',
    '',
    '## Done',
    '',
  ].join('\n')

  it('keeps ids of columns, cards and checklist items that are still there', () => {
    const previous = parseKanban(previousText)
    // Saved elsewhere: Build moved to Done, a card and a checklist item added.
    const next = parseKanban([
      '## To do',
      '- Design',
      '  - [ ] sketch',
      '  - [ ] new step',
      '  - [x] review',
      '- Added',
      '',
      '## Done',
      '- Build',
      '',
    ].join('\n'))
    adoptBoardIds(previous, next)

    expect(next.columns.map((column) => column.id)).toEqual(previous.columns.map((column) => column.id))
    expect(cardNamed(next, 'Design').id).toBe(cardNamed(previous, 'Design').id)
    expect(cardNamed(next, 'Build').id).toBe(cardNamed(previous, 'Build').id)

    const previousIds = new Set(previous.columns.flatMap((column) => column.cards).map((card) => card.id))
    expect(previousIds.has(cardNamed(next, 'Added').id)).toBe(false)

    const [sketch, newStep, review] = cardNamed(next, 'Design').checklist
    const [previousSketch, previousReview] = cardNamed(previous, 'Design').checklist
    expect(sketch!.id).toBe(previousSketch!.id)
    expect(review!.id).toBe(previousReview!.id)
    expect([previousSketch!.id, previousReview!.id]).not.toContain(newStep!.id)
  })

  it('gives a renamed card a new id', () => {
    const previous = parseKanban(previousText)
    const next = parseKanban(previousText.replace('- Build', '- Build it'))
    adoptBoardIds(previous, next)
    expect(cardNamed(next, 'Build it').id).not.toBe(cardNamed(previous, 'Build').id)
  })
})

describe('the open card in the URL', () => {
  // Two cards called Fix, in different columns, and an untitled one.
  const board = parseKanban(['## To do', '- Fix', '- Write docs', '- ', '', '## Done', '- Fix', ''].join('\n'))
  const [firstFix, docs, untitled] = board.columns[0]!.cards
  const secondFix = board.columns[1]!.cards[0]!

  it('names a card by its title, and which card of that title it is in board order', () => {
    expect(locateCard(board, docs!.id)).toEqual({ title: 'Write docs', occurrence: 1 })
    expect(locateCard(board, firstFix!.id)).toEqual({ title: 'Fix', occurrence: 1 })
    expect(locateCard(board, secondFix.id)).toEqual({ title: 'Fix', occurrence: 2 })
    expect(locateCard(board, untitled!.id)).toEqual({ title: '', occurrence: 1 })
  })

  it('names no card for an id not on the board', () => {
    expect(locateCard(board, -1)).toBeNull()
  })

  it('finds the card it names again', () => {
    expect(findCard(board, { title: 'Fix', occurrence: 2 })).toBe(secondFix)
    expect(findCard(board, { title: '', occurrence: 1 })).toBe(untitled)
  })

  it('finds nothing for a title or occurrence the board lacks, rather than another card', () => {
    expect(findCard(board, { title: 'Renamed', occurrence: 1 })).toBeNull()
    expect(findCard(board, { title: 'Fix', occurrence: 3 })).toBeNull()
  })

  it('reads the card from the query', () => {
    expect(cardLocatorFrom({ preview: 'a.kanban' })).toBeNull()
    expect(cardLocatorFrom({ card: 'Fix' })).toEqual({ title: 'Fix', occurrence: 1 })
    expect(cardLocatorFrom({ card: 'Fix', card_n: '2' })).toEqual({ title: 'Fix', occurrence: 2 })
    expect(cardLocatorFrom({ card: null })).toEqual({ title: '', occurrence: 1 })
  })

  it('reads a card_n that is not a whole number from 1 up as the first card', () => {
    for (const bad of ['0', '-2', '1.5', 'x', '']) {
      expect(cardLocatorFrom({ card: 'Fix', card_n: bad })).toEqual({ title: 'Fix', occurrence: 1 })
    }
  })

  it('writes the card into the query, keeping the other keys, and card_n only past the first', () => {
    const query = { path: 'work', preview: 'work/a.kanban' }
    expect(withCardLocator(query, { title: 'Fix', occurrence: 1 })).toEqual({ ...query, card: 'Fix' })
    expect(withCardLocator(query, { title: 'Fix', occurrence: 2 })).toEqual({ ...query, card: 'Fix', card_n: '2' })
    expect(withCardLocator(query, { title: '', occurrence: 1 })).toEqual({ ...query, card: '' })
  })

  it('removes both keys when closing', () => {
    expect(withCardLocator({ preview: 'a.kanban', card: 'Fix', card_n: '2' }, null)).toEqual({ preview: 'a.kanban' })
  })

  it('keeps the open card while the file browser goes on showing the same board', () => {
    const previous = { path: 'work', preview: 'work/a.kanban', card: 'Fix', card_n: '2' }
    expect(withCardKept({ path: 'work', preview: 'work/a.kanban', q: 'fi' }, previous))
      .toEqual({ path: 'work', preview: 'work/a.kanban', q: 'fi', card: 'Fix', card_n: '2' })
  })

  it('drops the open card when the file browser shows another file, the board as text, or nothing', () => {
    const previous = { preview: 'work/a.kanban', card: 'Fix' }
    expect(withCardKept({ preview: 'work/b.kanban' }, previous)).toEqual({ preview: 'work/b.kanban' })
    expect(withCardKept({ preview: 'work/a.kanban', view: 'text' }, previous)).toEqual({ preview: 'work/a.kanban', view: 'text' })
    expect(withCardKept({ path: 'work' }, previous)).toEqual({ path: 'work' })
  })

  it('reads back what it writes', () => {
    for (const locator of [{ title: 'Fix', occurrence: 2 }, { title: 'a & b?', occurrence: 1 }, { title: '', occurrence: 1 }]) {
      expect(cardLocatorFrom(withCardLocator({}, locator))).toEqual(locator)
    }
  })
})
