import { describe, expect, it, vi } from 'vitest'
import {
  adoptBoardIds,
  buildCardMenu,
  cardLocatorFrom,
  columnName,
  duplicateCard,
  dueStatus,
  findCard,
  findCardWithColumn,
  locateCard,
  makeColumn,
  moveCard,
  parseKanban,
  removeCard,
  serializeKanban,
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

describe('a due date marked done', () => {
  const cardOf = (text: string) => parseKanban(`## To do\n${text}\n`).columns[0]!.cards[0]!

  it('is read from @done on the title line, wherever it sits among the other marks', () => {
    for (const line of ['- Ship #ops @2026-11-01 @done', '- Ship #ops @done @2026-11-01']) {
      expect(cardOf(line)).toMatchObject({ title: 'Ship', labels: ['ops'], due: '2026-11-01', dueDone: true })
    }
    expect(cardOf('- Ship @2026-11-01').dueDone).toBe(false)
  })

  it('is written as @done after the date', () => {
    const board = parseKanban('## To do\n- Ship #ops @2026-11-01\n')
    board.columns[0]!.cards[0]!.dueDone = true
    expect(serializeKanban(board)).toBe('## To do\n- Ship #ops @2026-11-01 @done\n')
  })

  it('keeps a second @done as part of the title, as with a second date', () => {
    expect(cardOf('- Say @done @done')).toMatchObject({ title: 'Say @done', dueDone: true })
  })

  it("keeps a hand-written @done that has no date, so nothing in the file is dropped", () => {
    const text = '## To do\n- Ship @done\n'
    expect(cardOf('- Ship @done')).toMatchObject({ title: 'Ship', due: null, dueDone: true })
    expect(serializeKanban(parseKanban(text))).toBe(text)
  })

  it('is done rather than overdue, however long ago it was due', () => {
    expect(dueStatus('2000-01-01')).toBe('overdue')
    expect(dueStatus('2000-01-01', true)).toBe('done')
    expect(dueStatus('2999-01-01', true)).toBe('done')
  })
})

describe('board edits', () => {
  const makeBoard = () => parseKanban([
    '## To do', '- Design #ui @2026-11-01', '  - [x] sketch', '  - [ ] review', '- Build', '',
    '## Done', '- Ship', '',
  ].join('\n'))
  const titles = (board: KanbanBoard) => board.columns.map((column) => column.cards.map((card) => card.title))

  it('finds a card with the column it sits in', () => {
    const board = makeBoard()
    const ship = board.columns[1]!.cards[0]!
    expect(findCardWithColumn(board, ship.id)).toEqual({ card: ship, column: board.columns[1] })
    expect(findCardWithColumn(board, -1)).toBeNull()
  })

  it('removes a card', () => {
    const board = makeBoard()
    removeCard(board, board.columns[0]!.cards[1]!.id)
    expect(titles(board)).toEqual([['Design'], ['Ship']])
  })

  it('moves a card to the end of another column', () => {
    const board = makeBoard()
    moveCard(board, board.columns[0]!.cards[0]!.id, board.columns[1]!.id)
    expect(titles(board)).toEqual([['Build'], ['Ship', 'Design']])
  })

  it("leaves a card where it is when moved to its own column, or when the card or column isn't there", () => {
    const board = makeBoard()
    const design = board.columns[0]!.cards[0]!
    moveCard(board, design.id, board.columns[0]!.id)
    moveCard(board, design.id, -1)
    moveCard(board, -1, board.columns[1]!.id)
    removeCard(board, -1)
    expect(titles(board)).toEqual([['Design', 'Build'], ['Ship']])
  })

  it('puts a duplicate right below the card, with new ids and nothing shared', () => {
    const board = makeBoard()
    const design = board.columns[0]!.cards[0]!
    const duplicate = duplicateCard(board, design.id)!

    expect(titles(board)).toEqual([['Design', 'Design', 'Build'], ['Ship']])
    expect(board.columns[0]!.cards[1]).toBe(duplicate)
    expect(duplicate).toMatchObject({ title: 'Design', labels: ['ui'], due: '2026-11-01' })
    expect(duplicate.checklist.map((item) => [item.text, item.done])).toEqual([['sketch', true], ['review', false]])
    expect(duplicate.id).not.toBe(design.id)
    expect(duplicate.checklist[0]!.id).not.toBe(design.checklist[0]!.id)

    duplicate.labels.push('duplicate')
    duplicate.checklist[1]!.done = true
    expect(design.labels).toEqual(['ui'])
    expect(design.checklist[1]!.done).toBe(false)
  })

  it('duplicates nothing for a card not on the board', () => {
    const board = makeBoard()
    expect(duplicateCard(board, -1)).toBeNull()
    expect(titles(board)).toEqual([['Design', 'Build'], ['Ship']])
  })

  it("names an untitled column so it can be told apart in lists", () => {
    expect(columnName(makeColumn('Doing'))).toBe('Doing')
    expect(columnName(makeColumn(''))).toBe('Untitled column')
  })
})

describe('buildCardMenu', () => {
  const board = parseKanban(['## To do', '- Design @2026-11-01', '- Build', '', '## ', '', '## Done', '- Ship @2026-10-01 @done', ''].join('\n'))
  const [design, build] = board.columns[0]!.cards
  const ship = board.columns[2]!.cards[0]!
  const makeActions = () => ({ open: vi.fn(), toggleDueDone: vi.fn(), moveTo: vi.fn(), duplicate: vi.fn(), delete: vi.fn() })
  const labels = (cardId: number) =>
    buildCardMenu(board, cardId, makeActions()).map((item) => (item.separator ? '—' : item.label))

  it('offers Open, Move to each other column, Duplicate and Delete', () => {
    expect(labels(build!.id)).toEqual(['Open', '—', 'Move to Untitled column', 'Move to Done', '—', 'Duplicate', 'Delete'])
  })

  it('offers to mark the due date done, or not done, when the card has one', () => {
    expect(labels(design!.id)).toContain('Mark done')
    expect(labels(ship.id)).toContain('Mark not done')
    expect(labels(build!.id).some((label) => label?.startsWith('Mark'))).toBe(false)
  })

  it('marks Delete as dangerous', () => {
    const items = buildCardMenu(board, build!.id, makeActions())
    expect(items.find((item) => item.label === 'Delete')!.danger).toBe(true)
  })

  it('runs each action on the card, and moves to the column chosen', () => {
    const actions = makeActions()
    const items = buildCardMenu(board, design!.id, actions)
    for (const item of items) item.action?.()

    expect(actions.open).toHaveBeenCalledWith(design)
    expect(actions.toggleDueDone).toHaveBeenCalledWith(design)
    expect(actions.moveTo.mock.calls).toEqual([[design, board.columns[1]!.id], [design, board.columns[2]!.id]])
    expect(actions.duplicate).toHaveBeenCalledWith(design)
    expect(actions.delete).toHaveBeenCalledWith(design)
  })

  it('is empty for a card no longer on the board', () => {
    expect(buildCardMenu(board, -1, makeActions())).toEqual([])
  })
})
