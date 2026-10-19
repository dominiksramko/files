// .kanban boards are Markdown, so they stay readable and editable by hand:
//
//   ## To do
//   - Design login page #frontend @2026-11-01
//     Needs a dark mode variant.
//     - [x] wireframe
//     - [ ] build component
//
// "## " starts a column and an unindented "- " starts a card. Trailing #labels
// and an @YYYY-MM-DD due date are lifted off the card's title line. Indented
// lines belong to the card: "- [ ]" / "- [x]" are checklist items, anything
// else is the card's Markdown description. Nothing in a hand-edited file is
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
  return { id: nextId++, title, description: '', labels: [], due: null, checklist: [] }
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

export function dueStatus(due: string): 'overdue' | 'today' | 'upcoming' {
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
const TRAILING_META_RE = new RegExp(`(?:^|\\s+)(#${LABEL_PATTERN}|@\\d{4}-\\d{2}-\\d{2})$`, 'u')

function parseTitleLine(text: string): Pick<KanbanCard, 'title' | 'labels' | 'due'> {
  let title = text.trim()
  const labels: string[] = []
  let due: string | null = null
  for (let match = TRAILING_META_RE.exec(title); match; match = TRAILING_META_RE.exec(title)) {
    const token = match[1]!
    if (token.startsWith('@')) {
      if (due !== null) break // a second date stays part of the title
      due = token.slice(1)
    } else {
      labels.unshift(token.slice(1))
    }
    title = title.slice(0, match.index).trimEnd()
  }
  return { title, labels, due }
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
      if (checklistMatch) {
        card.checklist.push(makeChecklistItem((checklistMatch[2] ?? '').trim(), checklistMatch[1] !== ' '))
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
  ].filter(Boolean).join(' ')

  // Drop leading/trailing blank lines but keep indentation inside the description.
  const description = card.description.replace(/^(?:[ \t]*\n)+/, '').replace(/(?:\n[ \t]*)+$/, '')
  const descriptionLines = description.trim()
    ? description.split('\n').map((line) => (line.trim() ? `  ${line.trimEnd()}` : ''))
    : []

  const checklistLines = card.checklist.map((item) =>
    `  - [${item.done ? 'x' : ' '}] ${singleLine(item.text)}`.trimEnd(),
  )

  return [titleLine, ...descriptionLines, ...checklistLines]
}

export function serializeKanban(board: KanbanBoard): string {
  const sections = board.columns.map((column) =>
    [`## ${singleLine(column.title)}`.trimEnd(), ...column.cards.flatMap(serializeCard)].join('\n'),
  )
  if (board.preamble) sections.unshift(board.preamble)
  return sections.length ? sections.join('\n\n') + '\n' : ''
}
