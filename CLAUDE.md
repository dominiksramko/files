# Project — files app

## What This Is
A self-hosted file browser with Vue3 frontend + Go backend.

## Code Quality Rules (IMPORTANT — applies to both frontend and backend)
- **No duplication.** Before writing new logic, check if it already exists. If the same logic appears in more than one place, extract it — composable, utility function, helper, middleware, shared component, whatever fits.
- **Refactor proactively.** If a user mentions that something is used elsewhere, or if you notice while implementing that near-identical code already exists, do the refactor as part of the task — don't just copy-paste and move on.
- **Shared logic belongs in one place.** Event handlers, formatters, API calls, drag-and-drop, conflict resolution, Go helper functions — if two places do the same thing, they should call the same function.
- **Descriptive variable names.** No single-letter variables or unclear abbreviations (`e`, `p`, `m`, `val`, `tmp`, etc.). Names should convey meaning at a glance.

## Key Paths
- `backend/` — Go + Gin + SQLite API server
- `frontend/` — Vue3 + Pinia + Tailwind SPA
- `docker-compose.yml` — production deployment
- `docker-compose.dev.yml` — dev stack (backend `dev` stage of `backend/Dockerfile` with air live reload via `backend/.air.toml`, + Vite on :5173). Run tooling inside it: `docker compose -f docker-compose.dev.yml exec backend go test ./...`, `... exec frontend npm run lint`

## Backend
- Entry: `backend/cmd/server/main.go`
- Config: `backend/internal/config/config.go` (env vars: PORT, JWT_SECRET, DB_PATH, USERS_ROOT, GIN_MODE, THUMB_CACHE_DIR, HLS_CACHE_DIR, CONV_CACHE_DIR, ADMIN_USERNAME, ADMIN_PASSWORD, ADMIN_PATH)
- Auth middleware: `backend/internal/middleware/auth.go` — accepts JWT from `Authorization: Bearer` header OR `?token=` query param
- Handlers: `backend/internal/handlers/{auth,files,users,archive}.go`
- DB: SQLite via `modernc.org/sqlite` (pure Go, no CGO); auto-seeds admin user on first run — credentials configurable via `ADMIN_USERNAME` (default: `admin`), `ADMIN_PASSWORD` (random if unset, printed to logs), `ADMIN_PATH` (optional root dir)
- API prefix: `/api/v1`

## Database Migrations (IMPORTANT)
- Migration files: `backend/internal/database/migrations/YYYY_MM_DD_HHMMSS_name.go`
- Registry: `backend/internal/database/migrations/registry.go` — defines `Migration` struct, `Register()`, `All()`
- Runner: `backend/internal/database/db.go` → `runMigrations()` — runs on every server start
- Tracking table: `migrations` (version TEXT PK, name TEXT, applied_at TEXT)
- Each migration file has `Up` and `Down` funcs; runs in a transaction
- **To create a new migration:** `go run ./cmd/migrate <name>` from `backend/` dir
  - e.g. `go run ./cmd/migrate add_email_to_users`
  - Generates a file with the current timestamp + name, with empty Up/Down stubs
- **Never edit or delete existing migration files** — only append new ones

## Frontend
- Stack: Vue3 + TypeScript + Pinia + Vue Router v5 + Tailwind v4
- API client: `frontend/src/utils/api.ts` (uses fetch, attaches Bearer token)
- Stores: `auth.ts` (login/logout, user), `files.ts` (current path + directory listing)
- Types: `frontend/src/types/index.ts`
- Dev proxy: Vite proxies `/api` → `http://localhost:8080`
- Live folder updates: `GET /files/watch` (SSE, `backend/internal/handlers/watch.go`) starts with a `snapshot` event (the full listing, read after the watcher is live), resends one whenever events may have been lost, and pings every 25 s. `stores/files.ts` loads folders from that snapshot, reconnects with backoff (and on silence, tab visible, `online`), so every reconnect resyncs. After writing a file, don't call `filesStore.refresh()` — the stream reports the change; `refresh()` reopens the stream.

## Project Structure Convention
- `components/ui/` — generic reusable (AppModal, AppSpinner, AppCheckbox)
- `components/layout/` — shell components (AppLayout, AppHeader)
- `components/files/` — file-specific (FileList, FileIcon, FileBreadcrumb, FilePreview)
- `components/archive/` — ArchiveContents
- `components/admin/` — UserFormModal
- `views/` — page-level (LoginView, FilesView, AdminView, SettingsView)

## UI Component Rules
- **AppCheckbox.vue** — always use for checkboxes; supports `v-model`, `indeterminate` prop; never use native `<input type="checkbox">`
- **SecretInput.vue** — ALWAYS use for passwords that aren't the user's account password (archive passwords); NEVER a native `type="password"` there — password managers autofill the login into those and offer to save what's typed. Supports `v-model`; other attributes fall through. Account password fields stay `type="password"` with the right `autocomplete` (`current-password` / `new-password`).
- **AppSelect.vue** — ALWAYS use for dropdowns/selects; NEVER use native `<select>`. Supports `v-model`, `options: { label, value }[]`, `disabled`. Handles string and number values correctly. Usage: `<AppSelect v-model="val" :options="[{ label: 'Foo', value: 'foo' }]" />`
- **AppAccordion.vue + AppAccordionItem.vue** — use for collapsible settings sections; only one item open at a time. Wrap items in `<AppAccordion>`, each section in `<AppAccordionItem title="…">`. Uses CSS grid-template-rows transition for smooth animation.
- **AppModal.vue** — for larger complex modals with header X-button, title, footer slot; sizes sm/md/lg/xl/2xl
- **PresetPill.vue** (`components/files/`) — full-width preset list row with left accent border when active, inline delete confirm; used in AdvancedRenamer presets popover. Props: `name`, `template`, `active` (blue left border + bg when current template matches). Emits: `select`, `delete`.
- **AppIcon.vue** — ALWAYS use for icons; NEVER write inline SVGs. Registry: `frontend/src/utils/icons.ts`. Usage: `<AppIcon name="trash" class="size-4" />`. Optional `stroke-width` prop. To add a new icon, add it to `icons.ts` first.
- **useToast** — ALWAYS use for user-facing messages (success, error, info). NEVER use `alert()`. Import: `import { useToast } from '@/composables/useToast'`. Usage: `const { show: showToast } = useToast()` then `showToast('message', 'success' | 'error' | 'info')`. `AppToast` is mounted globally in `AppLayout` — no need to add it per-view.
- **AppButton.vue** — ALWAYS use for modal/dialog buttons; NEVER write raw `<button>` elements with inline Tailwind classes in modals. Props: `variant` (`'primary'` | `'destructive'` | `'secondary'` | `'ghost'`, default ghost), `disabled`, `type`. Usage: `<AppButton variant="primary" @click="confirm">Save</AppButton>`. `disabled` automatically adds `opacity-50 cursor-not-allowed`.

## Editable File Types Convention (IMPORTANT)
File types with their own editor (`.todo` → `TodoViewer`, `.kanban` → `KanbanViewer`) follow one pattern — reuse it for new ones:
- **Load + auto-save:** `useEditableFile(() => props.entry, { state, parse, serialize, onLoaded?, carryOverIds? })` from `frontend/src/composables/useFileContent.ts` — parses the file into `state`, auto-saves the serialized state 800 ms after each change. Read-only loading (e.g. `TextViewer`) uses `useFileContent` from the same file.
- **Simultaneous editing:** saves are versioned — `GET /files/content` returns the version as `ETag`, `PUT /files/content` with `If-Match` answers 412 (`EditConflictError` in `api.ts`) with the current content when the file changed. `useEditableFile` merges both that and changes reported by the watch stream into the editor with `mergeText()` (`utils/merge.ts`, local version wins on overlap); `TextViewer` merges on save and falls back to conflict markers. Pass `carryOverIds` (built on `adoptIds()`) so re-parsed entries keep their ids and open dialogs/focus survive a merge. Never write editor content without the base version.
- **Loading / error / too-large UI:** wrap the editor in `<EditableFileState>` (`components/files/`).
- **Registration:** `isXFile()` + category in `utils/formatters.ts` (classified before the mime checks), the view's name in `customViewName()` (enables the preview header's "View as text" toggle and the "Open as text" file op), `FileCategory` in `types/index.ts`, icon in `icons.ts`, viewer in `FilePreview.vue` `viewerFor()`, and an entry in `newFileTypes` in `FilesView.vue` — the single list behind the toolbar, mobile menu and context-menu "New …" items (they create the file with `fileops.create`, which never overwrites an existing one). Backend `mimeByExt` maps the extension to `text/plain`.
- **Checklists:** `useChecklistEditor` (`composables/`) + `TodoRow.vue` for any Enter/Backspace-driven checkbox list.
- **Drag and drop:** `vue-draggable-plus` with `:force-fallback="true"`, `ghost-class="drag-ghost"`, `fallback-class="drag-fallback"` (global styles in `src/style.css`).
- **Markdown:** render with `renderMarkdown(source, entryPath)` from `utils/markdown.ts` inside a `.markdown-body` element.
- **Horizontal gestures:** mark regions that scroll/drag sideways with `data-no-carousel-swipe` so the preview carousel doesn't treat them as prev/next swipes. Don't `.stop` touch events instead — SortableJS listens on `document`.
- **Escape inside a viewer:** if an Escape handler removes the focused element, use `@keydown.escape.stop`, otherwise the preview's window listener sees focus on `<body>` and closes the preview.

## Debounce Convention
Always use `debounce()` from `frontend/src/utils/debounce.ts` — never write raw `clearTimeout`/`setTimeout` pairs. Returns a debounced function with a `.cancel()` method; call `.cancel()` in `onUnmounted` / watch cleanup to avoid stale timers. Usage: `const debouncedFn = debounce(fn, delayMs)`.

## Header Center Slot Convention (IMPORTANT)
`AppHeader` is a layout shell — it must never import or reference view-specific components directly. It renders a generic center slot via `layout.headerCenter: shallowRef<Component | null>` from the layout store.

Views that need to render content in the header create a `defineComponent` whose render function closes over their own reactive state, set it on `layoutStore.headerCenter`, and clear it in `onUnmounted`. AppHeader renders it blindly with `<component :is="layout.headerCenter">`.

Currently used only by `FilesView` to render `FileBreadcrumb` in place of the logo. The pattern is generic — any future view can use it.

## File Operations Convention (IMPORTANT)
When adding a new file operation, add it ONLY to `buildFileOps()` in `frontend/src/composables/useFileOps.ts` — it is the single source of truth. The context menu (`contextMenuItems`) and toolbar (`FileOpsBar` via `fileOps` computed) both derive from it automatically. Each op is `{ key, label, icon, danger?, visible?, disabled?, action }`. Use `visible` for conditional ops (e.g. single-selection only).

`FilesView.vue` uses `useFileOps()` to get `clipboard`, `fileOps`, `contextMenuItems`, `onContextMenu`, `onPaste`, and `performMove`. New action callbacks (e.g. opening a dialog) must be passed via `actionCallbacks` when calling `useFileOps`.

## URL State Convention (IMPORTANT)
ALL browsing state must be reflected in the URL so page refresh restores it:
- `?path=` — current folder (synced via `navigate()` + `watch(route.query.path)`)
- `?preview=` — open preview file path
- `?q=` — search query
- `?view=text` — current preview shown as raw text instead of its custom view (`.todo`, `.kanban`)

Any new state that should survive refresh MUST be added to the URL and synced bidirectionally with `watch(route.query.xxx)` + `watch(stateRef)`.

## Modal Design Rules (FilesView.vue inline modals)
- Wrapper: `<Transition name="fade"><div v-if="..." class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" @click.self="close">`
- Card: `bg-zinc-900 border border-zinc-700 rounded-xl p-5 shadow-2xl`
- Width: `w-72` for simple confirm/input; `max-w-sm` for modals with lists or more content
- Title: `<h3 class="text-sm font-semibold text-zinc-100 mb-3">`
- Body text/description: `<p class="text-xs text-zinc-400 mb-3">`
- Text input: `w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors`
- Key handlers on inputs: `@keydown.enter` to confirm, `@keydown.escape` to close

## File Conflict Resolution Convention
When a file operation (move, copy, upload, extract, compress, new file) would overwrite an existing file, the backend detects the conflict and returns HTTP 409 with `{ conflicts: string[] }` (list of conflicting filenames). The frontend then presents the user with a resolution modal offering:
- **Replace** — overwrite the existing file(s)
- **Keep all** — rename the incoming file(s) by appending a suffix (e.g. `file (1).txt`, `file (2).txt`) until no conflict remains
- **Cancel** — abort the operation entirely

The resolution choice is sent back to the backend as `conflict_mode: 'replace' | 'rename'` on the retry request.

### Unified conflict dialog system (IMPORTANT)
All conflict flows use a single shared composable + component — do NOT create per-operation conflict dialogs:
- Composable: `frontend/src/composables/useConflictDialog.ts` — `prompt(title, names): Promise<'replace' | 'rename' | null>`
- Component: `frontend/src/components/ui/ConflictDialog.vue` — mounted once globally in `FilesView.vue` via `<ConflictDialog />`
- Usage: `const { prompt: promptConflict } = useConflictDialog()` then `const choice = await promptConflict('Title', conflictNames)` — returns `'replace'`, `'rename'`, or `null` (cancel)
- Backend `conflict_mode` parameter: `'replace'` maps to replace, `'rename'` maps to keep-all/rename

## External Tool Dependencies (must be in Dockerfile `runtime-base` stage)
Any new CLI tool used by the backend must be added to the `apk add` line in the `runtime-base` stage of `backend/Dockerfile` (Alpine) — both the production runtime and the dev container build on it. Current tools:
- `ffmpeg` — video thumbnails + HLS transcoding
- `p7zip` — 7z/RAR archive support
- `rsvg-convert` (Alpine package `rsvg-convert`) — SVG rasterisation for thumbnails
- `imagemagick` + `imagemagick-heic` — thumbnails for AVIF, HEIC, HEIF, JXL, ICO, PSD, TGA, XCF, HDR, EXR, DDS
- `poppler-utils` — PDF utilities (e.g. `pdftoppm` for PDF thumbnails)
- `unrar` — RAR extraction; copied from `linuxserver/unrar:7.2.4` image as `/usr/bin/unrar-alpine → /usr/bin/unrar` (not an apk package)
- `ca-certificates` — TLS root certificates
- `tzdata` — timezone data

