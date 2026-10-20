import { describe, expect, it } from 'vitest'
import { adoptBoardIds, parseKanban, type KanbanBoard } from '@/utils/kanban'

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
