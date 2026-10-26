import type { LocationQuery } from 'vue-router'
import type { ContextMenuItem } from '@/components/ui/ContextMenu.vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { fileCategory } from '@/utils/formatters'
import { adoptIds } from '@/utils/merge'
import { nameOf, parentOf, relativePath, resolvePath } from '@/utils/paths'
import { queryString, withKeys } from '@/utils/query'

// .kanban boards are Markdown, so they stay readable and editable by hand:
//
//   ## To do
//   - Design login page #frontend @2026-11-01
//     Needs a dark mode variant.
//     @file Roadmap files/mockup.png
//     - [x] wireframe
//     - [ ] build component
//
// "## " starts a column and an unindented "- " starts a card. Trailing #labels
// and an @YYYY-MM-DD due date (with @done once it's done) are lifted off the
// card's title line. Indented lines belong to the card: "- [ ]" / "- [x]" are
// checklist items, "@file <path>" attaches a file (the path relative to the
// board's folder, see resolveAttachment), anything else is the card's Markdown
// description. Nothing in a hand-edited file is
// dropped: text before the first column is kept as a preamble, and a stray
// line inside a column becomes a card.

export interface KanbanChecklistItem {
  id: number
  text: string
  done: boolean
}

export interface KanbanCard {
  id: number
  title: string
  description: string
  labels: string[]
  due: string | null // YYYY-MM-DD
  dueDone: boolean // the due date is marked done: "@done" on the title line
  attachments: string[] // links to files, as written after "@file"
  checklist: KanbanChecklistItem[]
}

export interface KanbanColumn {
  id: number
  title: string
  cards: KanbanCard[]
}

export interface KanbanBoard {
  preamble: string
  columns: KanbanColumn[]
}

// Ids only key Vue lists and drag-and-drop; they're never written to the file.
let nextId = 1

export function makeChecklistItem(text: string, done = false): KanbanChecklistItem {
  return { id: nextId++, text, done }
}

export function makeCard(title: string): KanbanCard {
  return { id: nextId++, title, description: '', labels: [], due: null, dueDone: false, attachments: [], checklist: [] }
}

export function makeColumn(title: string): KanbanColumn {
  return { id: nextId++, title, cards: [] }
}

export const DEFAULT_COLUMNS = ['To do', 'Doing', 'Done']

// ── Labels ───────────────────────────────────────────────────────────────────
// A label starts with a letter, so "Fix bug #123" keeps "#123" in its title.
const LABEL_PATTERN = String.raw`\p{L}[\p{L}\p{N}_/-]*`
const LABEL_RE = new RegExp(`^${LABEL_PATTERN}$`, 'u')

// Turns user input ("#Front end") into a valid label ("Front-end"), or null.
export function normalizeLabel(raw: string): string | null {
  const label = raw.trim().replace(/^#+/, '').replace(/\s+/g, '-')
  return LABEL_RE.test(label) ? label : null
}

// Colours aren't stored in the file; each label name always hashes to the same one.
const LABEL_COLORS = [
  'bg-red-500/20 text-red-300',
  'bg-orange-500/20 text-orange-300',
  'bg-amber-500/20 text-amber-300',
  'bg-lime-500/20 text-lime-300',
  'bg-emerald-500/20 text-emerald-300',
  'bg-cyan-500/20 text-cyan-300',
  'bg-sky-500/20 text-sky-300',
  'bg-indigo-500/20 text-indigo-300',
  'bg-violet-500/20 text-violet-300',
  'bg-pink-500/20 text-pink-300',
]

export function labelColor(label: string): string {
  let hash = 0
  for (const char of label.toLowerCase()) hash = (hash * 31 + char.codePointAt(0)!) >>> 0
  return LABEL_COLORS[hash % LABEL_COLORS.length]!
}

// ── Due dates ────────────────────────────────────────────────────────────────
function todayIso(): string {
  const now = new Date()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${now.getFullYear()}-${month}-${day}`
}

// A due date marked done is done whenever it was, never overdue.
export function dueStatus(due: string, done = false): 'done' | 'overdue' | 'today' | 'upcoming' {
  if (done) return 'done'
  const today = todayIso()
  return due < today ? 'overdue' : due === today ? 'today' : 'upcoming'
}

export function formatDue(due: string): string {
  const [year, month, day] = due.split('-').map(Number)
  const date = new Date(year!, month! - 1, day)
  return date.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: year === new Date().getFullYear() ? undefined : 'numeric',
  })
}

// ── Parse ────────────────────────────────────────────────────────────────────
const COLUMN_RE = /^##(?:\s+(.*))?$/
const CARD_RE = /^[-*](?:\s+(.*))?$/
const INDENTED_RE = /^(?: {2}|\t)(.*)$/
const CHECKLIST_RE = /^[-*]\s+\[([ xX])\](?:\s(.*))?$/
const ATTACHMENT_RE = /^@file\s+(\S.*)$/
const TRAILING_META_RE = new RegExp(`(?:^|\\s+)(#${LABEL_PATTERN}|@\\d{4}-\\d{2}-\\d{2}|@done)$`, 'u')

function parseTitleLine(text: string): Pick<KanbanCard, 'title' | 'labels' | 'due' | 'dueDone'> {
  let title = text.trim()
  const labels: string[] = []
  let due: string | null = null
  let dueDone = false
  for (let match = TRAILING_META_RE.exec(title); match; match = TRAILING_META_RE.exec(title)) {
    const token = match[1]!
    if (token === '@done') {
      if (dueDone) break // a second one stays part of the title
      dueDone = true
    } else if (token.startsWith('@')) {
      if (due !== null) break // a second date stays part of the title
      due = token.slice(1)
    } else {
      labels.unshift(token.slice(1))
    }
    title = title.slice(0, match.index).trimEnd()
  }
  return { title, labels, due, dueDone }
}

// Keeps columns, cards and checklist items that survive a reload or merge on
// their previous ids (matched by title / text), so the open card stays open.
export function adoptBoardIds(previous: KanbanBoard, next: KanbanBoard) {
  adoptIds(previous.columns, next.columns, (column) => column.title)
  adoptIds(
    previous.columns.flatMap((column) => column.cards),
    next.columns.flatMap((column) => column.cards),
    (card) => card.title,
    (previousCard, nextCard) => adoptIds(previousCard.checklist, nextCard.checklist, (item) => item.text),
  )
}

// ── Board edits (shared by the card editor and the card menu) ────────────────
export const columnName = (column: KanbanColumn) => column.title || 'Untitled column'

export function findCardWithColumn(board: KanbanBoard, cardId: number): { card: KanbanCard; column: KanbanColumn } | null {
  for (const column of board.columns) {
    const card = column.cards.find((candidate) => candidate.id === cardId)
    if (card) return { card, column }
  }
  return null
}

export function removeCard(board: KanbanBoard, cardId: number) {
  const found = findCardWithColumn(board, cardId)
  if (found) found.column.cards = found.column.cards.filter((card) => card.id !== cardId)
}

// Moves the card to the end of another column; nothing happens within its own.
export function moveCard(board: KanbanBoard, cardId: number, targetColumnId: number) {
  const found = findCardWithColumn(board, cardId)
  const target = board.columns.find((column) => column.id === targetColumnId)
  if (!found || !target || found.column === target) return
  found.column.cards = found.column.cards.filter((card) => card.id !== cardId)
  target.cards.push(found.card)
}

// Puts a duplicate of the card right below it and returns it (new ids, nothing shared).
export function duplicateCard(board: KanbanBoard, cardId: number): KanbanCard | null {
  const found = findCardWithColumn(board, cardId)
  if (!found) return null
  const { card, column } = found
  const duplicate: KanbanCard = {
    ...card,
    id: nextId++,
    labels: [...card.labels],
    attachments: [...card.attachments],
    checklist: card.checklist.map((item) => makeChecklistItem(item.text, item.done)),
  }
  column.cards.splice(column.cards.indexOf(card) + 1, 0, duplicate)
  return duplicate
}

// ── Attachments ──────────────────────────────────────────────────────────────
// A card links files anywhere in the user's folder, relative to the board's
// folder. Uploads go into a folder next to the board named after it
// ("Roadmap.kanban" → "Roadmap files"), made by the first upload. Removing an
// attachment, deleting its card or the board leaves the file where it is.

// Where uploads for the board at boardPath go.
export function attachmentFolder(boardPath: string): string {
  const stem = nameOf(boardPath).replace(/\.kanban$/i, '') || 'Board'
  return [parentOf(boardPath), `${stem} files`].filter(Boolean).join('/')
}

// The file an attachment link leads to; null when it can't lead anywhere.
export function resolveAttachment(boardPath: string, link: string): string | null {
  return resolvePath(parentOf(boardPath), link)
}

// How the board at boardPath links the file at path.
export function attachmentLink(boardPath: string, path: string): string {
  return relativePath(parentOf(boardPath), path)
}

// Attaches the files at paths to the card, each once.
export function addAttachments(card: KanbanCard, boardPath: string, paths: string[]) {
  const attached = new Set(card.attachments.map((link) => resolveAttachment(boardPath, link)))
  for (const path of paths) {
    if (attached.has(path)) continue
    attached.add(path)
    card.attachments.push(attachmentLink(boardPath, path))
  }
}

// The card's first attached image, shown as its cover; entryFor gives a
// file's details (null while unknown or when it's missing).
export function cardCover(
  card: KanbanCard,
  boardPath: string,
  entryFor: (path: string) => FileEntry | null | undefined,
): FileEntry | null {
  for (const link of card.attachments) {
    const path = resolveAttachment(boardPath, link)
    const entry = path === null ? null : entryFor(path)
    if (entry && !entry.is_dir && fileCategory(entry.mime, entry.name) === 'image') return entry
  }
  return null
}

// An attached image to show small: its thumbnail once one is ready, else the file.
export function attachmentImageUrl(entry: FileEntry): string {
  return entry.has_thumb && entry.thumb_hash
    ? filesApi.previewThumbUrl(entry.path, entry.thumb_hash)
    : filesApi.streamUrl(entry.path, entry.mod_time)
}

// ── Card menu (right-click or ⋯ on a card) ───────────────────────────────────
export interface CardMenuActions {
  open: (card: KanbanCard) => void
  toggleDueDone: (card: KanbanCard) => void
  moveTo: (card: KanbanCard, columnId: number) => void
  duplicate: (card: KanbanCard) => void
  delete: (card: KanbanCard) => void
}

// Open; Mark done / not done when the card has a due date; Move to each other
// column; Duplicate; Delete. Empty when the card isn't on the board (any more).
export function buildCardMenu(board: KanbanBoard, cardId: number, actions: CardMenuActions): ContextMenuItem[] {
  const found = findCardWithColumn(board, cardId)
  if (!found) return []
  const { card, column } = found
  const otherColumns = board.columns.filter((other) => other !== column)
  const items: ContextMenuItem[] = [{ label: 'Open', icon: 'pencilSquare', action: () => actions.open(card) }]
  if (card.due) {
    items.push({ label: card.dueDone ? 'Mark not done' : 'Mark done', icon: 'checkCircle', action: () => actions.toggleDueDone(card) })
  }
  if (otherColumns.length) {
    items.push({ separator: true })
    for (const other of otherColumns) {
      items.push({ label: `Move to ${columnName(other)}`, icon: 'arrowRight', action: () => actions.moveTo(card, other.id) })
    }
  }
  items.push(
    { separator: true },
    { label: 'Duplicate', icon: 'copy', action: () => actions.duplicate(card) },
    { label: 'Delete', icon: 'trash', danger: true, action: () => actions.delete(card) },
  )
  return items
}

// ── The open card in the URL ─────────────────────────────────────────────────
// Ids aren't written to the file, so the URL names the open card by its title
// (?card=Fix+login), adding which of the board's cards with that title it is
// when it isn't the first (&card_n=2). A link to a card renamed or deleted
// since then finds nothing rather than another card.
export interface CardLocator {
  title: string
  occurrence: number // 1: the board's first card with this title, in board order
}

const allCards = (board: KanbanBoard) => board.columns.flatMap((column) => column.cards)

// How the URL names the card with this id; null when it isn't on the board.
export function locateCard(board: KanbanBoard, cardId: number): CardLocator | null {
  const cards = allCards(board)
  const card = cards.find((candidate) => candidate.id === cardId)
  if (!card) return null
  const sameTitle = cards.filter((candidate) => candidate.title === card.title)
  return { title: card.title, occurrence: sameTitle.indexOf(card) + 1 }
}

// The card the URL names, or null when there's none like it.
export function findCard(board: KanbanBoard, locator: CardLocator): KanbanCard | null {
  return allCards(board).filter((card) => card.title === locator.title)[locator.occurrence - 1] ?? null
}

const cardKeys = ['card', 'card_n']

// The card query names, or null when none is open. An untitled card is a bare ?card.
export function cardLocatorFrom(query: LocationQuery): CardLocator | null {
  const title = queryString(query.card)
  if (title === undefined) return null
  const occurrence = Number(queryString(query.card_n) ?? 1)
  return { title, occurrence: Number.isInteger(occurrence) && occurrence >= 1 ? occurrence : 1 }
}

// query with locator's card open (null closes it); the other keys stay.
export function withCardLocator(query: LocationQuery, locator: CardLocator | null): LocationQuery {
  return withKeys(query, cardKeys, {
    card: locator?.title,
    card_n: locator && locator.occurrence > 1 ? String(locator.occurrence) : undefined,
  })
}

// For the file browser, which rebuilds its URL from what it shows: query with
// previous's open card kept while it shows the same board, as a board (not as
// text), and without one otherwise.
export function withCardKept(query: LocationQuery, previous: LocationQuery): LocationQuery {
  const sameBoard = !!query.preview && query.preview === previous.preview && query.view === undefined
  return withCardLocator(query, sameBoard ? cardLocatorFrom(previous) : null)
}

export function parseKanban(content: string): KanbanBoard {
  const preambleLines: string[] = []
  const columns: KanbanColumn[] = []
  let column: KanbanColumn | null = null
  let card: KanbanCard | null = null
  let descriptionLines: string[] = []
  let pendingBlankLines = 0

  function finishCard() {
    if (card) card.description = descriptionLines.join('\n')
    card = null
    descriptionLines = []
    pendingBlankLines = 0
  }

  for (const line of content.replace(/\r\n/g, '\n').split('\n')) {
    const columnMatch = COLUMN_RE.exec(line)
    if (columnMatch) {
      finishCard()
      column = makeColumn((columnMatch[1] ?? '').trim())
      columns.push(column)
      continue
    }
    if (!column) {
      preambleLines.push(line)
      continue
    }
    if (line.trim() === '') {
      if (card) pendingBlankLines++
      continue
    }

    const indentedMatch = INDENTED_RE.exec(line)
    if (indentedMatch && card) {
      const body = indentedMatch[1]!
      const checklistMatch = CHECKLIST_RE.exec(body)
      const attachmentMatch = ATTACHMENT_RE.exec(body)
      if (checklistMatch) {
        card.checklist.push(makeChecklistItem((checklistMatch[2] ?? '').trim(), checklistMatch[1] !== ' '))
      } else if (attachmentMatch) {
        card.attachments.push(attachmentMatch[1]!.trimEnd())
      } else {
        // Blank lines only matter between description lines (paragraph breaks).
        if (descriptionLines.length) descriptionLines.push(...Array<string>(pendingBlankLines).fill(''))
        descriptionLines.push(body)
      }
      pendingBlankLines = 0
      continue
    }

    // A new card: either "- title" or a stray line that we keep as a card.
    finishCard()
    const cardMatch = CARD_RE.exec(line)
    card = makeCard('')
    Object.assign(card, parseTitleLine(cardMatch ? (cardMatch[1] ?? '') : line))
    column.cards.push(card)
  }
  finishCard()

  return { preamble: preambleLines.join('\n').trim(), columns }
}

// ── Serialize ────────────────────────────────────────────────────────────────
// Titles and checklist items are single-line in the file.
function singleLine(text: string): string {
  return text.replace(/\s*\n\s*/g, ' ').trim()
}

function serializeCard(card: KanbanCard): string[] {
  const titleLine = [
    '-',
    singleLine(card.title),
    ...card.labels.map((label) => `#${label}`),
    card.due ? `@${card.due}` : '',
    card.dueDone ? '@done' : '',
  ].filter(Boolean).join(' ')

  // Drop leading/trailing blank lines but keep indentation inside the description.
  const description = card.description.replace(/^(?:[ \t]*\n)+/, '').replace(/(?:\n[ \t]*)+$/, '')
  const descriptionLines = description.trim()
    ? description.split('\n').map((line) => (line.trim() ? `  ${line.trimEnd()}` : ''))
    : []

  const attachmentLines = card.attachments.map((link) => `  @file ${singleLine(link)}`)

  const checklistLines = card.checklist.map((item) =>
    `  - [${item.done ? 'x' : ' '}] ${singleLine(item.text)}`.trimEnd(),
  )

  return [titleLine, ...descriptionLines, ...attachmentLines, ...checklistLines]
}

export function serializeKanban(board: KanbanBoard): string {
  const sections = board.columns.map((column) =>
    [`## ${singleLine(column.title)}`.trimEnd(), ...column.cards.flatMap(serializeCard)].join('\n'),
  )
  if (board.preamble) sections.unshift(board.preamble)
  return sections.length ? sections.join('\n\n') + '\n' : ''
}
