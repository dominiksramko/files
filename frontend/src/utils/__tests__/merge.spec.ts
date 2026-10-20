import { describe, expect, it } from 'vitest'
import { adoptIds, CONFLICT_MARKERS, mergeText } from '@/utils/merge'

const lines = (...items: string[]) => items.join('\n')

describe('mergeText', () => {
  it('takes the only side that changed', () => {
    expect(mergeText('a', 'a', 'b', 'mine')).toEqual({ text: 'b', conflicts: 0 })
    expect(mergeText('a', 'b', 'a', 'mine')).toEqual({ text: 'b', conflicts: 0 })
    expect(mergeText('a', 'b', 'b', 'markers')).toEqual({ text: 'b', conflicts: 0 })
  })

  it('merges edits to different lines', () => {
    const base = lines('one', 'two', 'three', 'four')
    const mine = lines('ONE', 'two', 'three', 'four')
    const theirs = lines('one', 'two', 'three', 'FOUR')
    expect(mergeText(base, mine, theirs, 'markers')).toEqual({ text: lines('ONE', 'two', 'three', 'FOUR'), conflicts: 0 })
  })

  it('merges edits to neighbouring lines without a conflict', () => {
    const base = lines('a', 'b', 'c', 'd')
    const mine = lines('a', 'B', 'c', 'd')
    const theirs = lines('a', 'b', 'C', 'd')
    expect(mergeText(base, mine, theirs, 'markers')).toEqual({ text: lines('a', 'B', 'C', 'd'), conflicts: 0 })
  })

  it('accepts the same edit made on both sides', () => {
    const base = lines('a', 'b', 'c', 'd')
    const theirs = lines('a', 'b', 'C', 'd')
    // Mine also changed another line, so the texts differ as a whole.
    const mine = lines('A', 'b', 'C', 'd')
    expect(mergeText(base, mine, theirs, 'markers')).toEqual({ text: lines('A', 'b', 'C', 'd'), conflicts: 0 })
  })

  it('keeps both insertions at the same spot, theirs first', () => {
    const base = lines('## To do', '- one', '')
    const mine = lines('## To do', '- one', '- mine', '')
    const theirs = lines('## To do', '- one', '- theirs', '')
    expect(mergeText(base, mine, theirs, 'markers')).toEqual({
      text: lines('## To do', '- one', '- theirs', '- mine', ''),
      conflicts: 0,
    })
  })

  it('keeps an insertion next to a line the other side changed', () => {
    const base = lines('a', 'b')
    const mine = lines('a', 'inserted', 'b')
    const theirs = lines('a', 'B')
    expect(mergeText(base, mine, theirs, 'markers')).toEqual({ text: lines('a', 'inserted', 'B'), conflicts: 0 })
  })

  it('keeps deletions from both sides', () => {
    const base = lines('a', 'b', 'c', 'd')
    const mine = lines('b', 'c', 'd')
    const theirs = lines('a', 'b', 'c')
    expect(mergeText(base, mine, theirs, 'markers')).toEqual({ text: lines('b', 'c'), conflicts: 0 })
  })

  describe('the same line changed differently on both sides', () => {
    const base = lines('ONE', 'two', 'three', 'FOUR', '')
    const mine = lines('ONE', 'bob two', 'three', 'FOUR', '')
    const theirs = lines('ONE', 'alice two', 'three', 'FOUR', '')

    it("keeps the local version with 'mine'", () => {
      expect(mergeText(base, mine, theirs, 'mine')).toEqual({ text: mine, conflicts: 1 })
    })

    it("puts both between conflict markers with 'markers'", () => {
      expect(mergeText(base, mine, theirs, 'markers')).toEqual({
        text: lines(
          'ONE',
          CONFLICT_MARKERS.mine, 'bob two', CONFLICT_MARKERS.separator, 'alice two', CONFLICT_MARKERS.theirs,
          'three', 'FOUR', '',
        ),
        conflicts: 1,
      })
    })
  })

  it('counts a deletion against an edit of the same line as a conflict', () => {
    const base = lines('a', 'b', 'c')
    const mine = lines('a', 'c')
    const theirs = lines('a', 'B', 'c')
    expect(mergeText(base, mine, theirs, 'mine')).toEqual({ text: mine, conflicts: 1 })
    expect(mergeText(base, mine, theirs, 'markers').text).toBe(
      lines('a', CONFLICT_MARKERS.mine, CONFLICT_MARKERS.separator, 'B', CONFLICT_MARKERS.theirs, 'c'),
    )
  })

  it("takes the whole overlapping region from mine with 'mine'", () => {
    // Mine changed lines 2–3, theirs lines 3–4: they overlap on line 3, so
    // lines 2–4 resolve as one region, the local version.
    const base = lines('1', '2', '3', '4', '5')
    const mine = lines('1', 'two', 'three', '4', '5')
    const theirs = lines('1', '2', 'THREE', 'FOUR', '5')
    expect(mergeText(base, mine, theirs, 'mine')).toEqual({ text: lines('1', 'two', 'three', '4', '5'), conflicts: 1 })
  })

  it('resolves separate regions independently', () => {
    const base = lines('a', 'b', 'c', 'd', 'e')
    const mine = lines('a', 'b', 'mine c', 'd', 'E')
    const theirs = lines('A', 'b', 'their c', 'd', 'e')
    // A and E merge cleanly; c conflicts.
    expect(mergeText(base, mine, theirs, 'mine')).toEqual({ text: lines('A', 'b', 'mine c', 'd', 'E'), conflicts: 1 })
  })
})

describe('adoptIds', () => {
  const items = (...texts: string[]) => texts.map((text, index) => ({ id: 100 + index, text }))

  it('gives matching entries their previous ids', () => {
    const previous = [{ id: 1, text: 'a' }, { id: 2, text: 'b' }]
    const next = items('b', 'new', 'a')
    adoptIds(previous, next, (item) => item.text)
    expect(next).toEqual([{ id: 2, text: 'b' }, { id: 101, text: 'new' }, { id: 1, text: 'a' }])
  })

  it('matches duplicates in order and never reuses an id', () => {
    const previous = [{ id: 1, text: 'same' }, { id: 2, text: 'same' }]
    const next = items('same', 'same', 'same')
    adoptIds(previous, next, (item) => item.text)
    expect(next.map((item) => item.id)).toEqual([1, 2, 102])
  })

  it('reports each match', () => {
    const matches: [number, number][] = []
    const next = items('a')
    adoptIds([{ id: 1, text: 'a' }], next, (item) => item.text, (previousItem, nextItem) => {
      matches.push([previousItem.id, nextItem.id])
    })
    expect(matches).toEqual([[1, 1]])
  })
})
