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
- `docker-compose.dev.yml` — dev stack (backend `dev` stage of `backend/Dockerfile` with air live reload via `backend/.air.toml`, + Vite on :5173). `dev.sh` drives it: `./dev.sh` starts it, `./dev.sh test` / `lint` / `migrate <name>` / `sh <service>` / `down`; anything else goes to `docker compose -f docker-compose.dev.yml`

## Versions
Releases are git tags `vMAJOR.MINOR.PATCH`; the server's version comes from `git describe` at build (`deploy.sh` → `APP_VERSION` → Dockerfile `VERSION` → `-ldflags -X main.version`), shown in Settings — never write a version number into the code. When a piece of work is done, say which number it raises since the last tag (`git describe --tags --abbrev=0`): **major** if updating needs a manual step or can't be undone (new required setting, a database change older versions can't read), **minor** for a new feature, **patch** for fixes only — and offer to tag it. Creating and pushing a tag needs the user's go-ahead, like a commit.

## Backend
- Entry: `backend/cmd/server/main.go`
- Config: `backend/internal/config/config.go` (env vars: PORT, JWT_SECRET, DB_PATH, USERS_ROOT, GIN_MODE, THUMB_CACHE_DIR, HLS_CACHE_DIR, CONV_CACHE_DIR, HLS_CACHE_MAX_GB, CONV_CACHE_MAX_GB, ADMIN_USERNAME, ADMIN_PASSWORD, ADMIN_PATH, TRUSTED_PROXIES)
- Production container runs the server as `PUID:PGID` (default 1000), never root: `backend/docker-entrypoint.sh` hands the database and caches to that user, then drops privileges with `su-exec`; `docker-entrypoint_test.sh` runs in the image build. Anything the server writes must live in `/data` (or `/tmp`).
- Login sessions: server-side (`backend/internal/sessions/`, table `sessions`); the browser holds only an HttpOnly, SameSite=Strict cookie (`files_session`), which it sends with fetches, images, video, downloads and EventSource alike — **never put credentials in URLs**. `middleware.Auth` looks the session up with the user's *current* details on every request (sets `userID`, `username`, `isAdmin`, `rootDir`, `sessionToken`). Sessions stay alive while used: 7 days unused or 30 days after login ends them; logout, a password change (other sessions) and deleting a user end them at once. `middleware.SameOrigin` refuses state-changing requests the browser marks as from another site. Failed logins are limited per client address (IPv6 per /64) and per username by `internal/loginlimit` (in memory), and logged with the address.
- Handlers: `backend/internal/handlers/{auth,files,users,archive}.go`
- Server-sent event streams (watch, search, sizes, disk usage scan) write through `startEventStream(c)` and `writeEvent(c, event, payload)` (`handlers/sse.go`; an empty event name sends an unnamed event). A long stream also stops on `h.streamsEnd` (server shutdown) and re-checks `sessionStillValid(c)` now and then.
- Deleting goes through `removeEntries(c, fsys, names)` (`fileops.go`), shared by `DELETE /files` and `DELETE /files/usage`: it reports already-missing items with a 404 before deleting anything. Deletes are permanent (there is no trash).
- Disk usage (`handlers/usage.go`, the disk usage modal over the file browser): `internal/diskusage` scans a folder through userfs (links not followed; the modal shows `Disk` = blocks with hard links counted once, folders included; the Details dialog shows both that and `Apparent` = file lengths) and keeps only folders plus the largest files on disk; a folder's files are read again when it's shown. Each user has one scan, kept in memory (`usageStore`, dropped after 30 min unused; at most 2 scans run at once server-wide; a new scan stops the user's running one). `diskusage.Count` measures the Details dialog's sizes (`GET /files/size`, a selection's hard links counted once across it).
- DB: SQLite via `modernc.org/sqlite` (pure Go, no CGO); auto-seeds admin user on first run — credentials configurable via `ADMIN_USERNAME` (default: `admin`), `ADMIN_PASSWORD` (random if unset, printed to logs), `ADMIN_PATH` (optional root dir)
- API prefix: `/api/v1`
- Responses that return a user's file inline (stream, content, office link) call `setUserFileHeaders(c, name)` first: `nosniff` plus a `Content-Security-Policy: sandbox`, so an HTML/SVG file opened directly can't run as part of the app (PDFs exempt — browsers won't show them sandboxed). New endpoints serving file bytes must do the same.

## Tests (IMPORTANT)
- **Every function you add or change gets tests in the same change** — the behaviour it's for, and the ways it must refuse (bad paths, conflicts, other users' files). For a bug fix, add a test that fails without the fix.
- Run them in the dev stack: `./dev.sh test` (both suites, then `npm run type-check`; `./dev.sh exec frontend npm run test:watch` while working). Vitest doesn't check types, but the production build does — `vue-tsc --build`, specs included — so a type error in a spec passes `npm test` and still stops the deploy.
- They gate production: the builder stages of `backend/Dockerfile` run `go vet` + `go test` and `npm test`, so `deploy.sh` stops before replacing the running containers if any fail.
- **Frontend:** Vitest, specs in a `__tests__/` folder next to the code (`src/utils/__tests__/merge.spec.ts`), named `*.spec.ts`. Keep logic in plain functions in `utils/` so it can be tested without mounting components.
- **Backend** helpers in `backend/internal/handlers/handlers_test.go`: `serve` / `serveJSON` call a handler as a logged-in user with a given home folder (no router or session needed); `sessionRouter` / `login` / `call` in `auth_test.go` go through the real session middleware; `internal/testutil` (`OpenDB`, `CreateUser`) gives any package a migrated database; `newUserFolders` creates neighbouring home folders `ann` and `anna`; `assertUnchanged`, `assertFileContent`, `assertMissing`, `assertStatus`, `decodeResponse`. Keep logic testable: pure helpers for decisions, handlers only for request/response.
- **User files only through `userfs` (IMPORTANT).** A handler gets the user's folder with `userFS(c)` / `requestFolderName` / `requestChildName(s)` (`handlers/files.go`) and passes *names relative to that folder* to its methods (Open, Create, ReadDir, Rename, RemoveAll, WalkDir, …). It's an `os.Root`: the OS keeps every operation inside the folder — `..` can't climb out, symbolic links work only while they stay inside, absolute links are refused. Never build a path from `userRoot(c)` and call `os.*` on it. `requestChildName` refuses the folder itself for anything that creates, replaces, renames, moves or deletes. Programs that need a path (ffmpeg, 7z, the file watcher, thumbnails) get `fsys.RealPath(name)`, which is checked the same way; they may read there, but only extraction tools write, and only into a fresh staging folder whose links are removed afterwards. App data (database, caches, `os.TempDir`) is outside the gate and uses `os` normally. `TestHandlersReachUserFilesThroughUserfs` (`handlers/userfs_guard_test.go`) fails on any handler function that touches files by path (`os.Open`, `os.Remove`, `c.File`, …) unless it's in its allow-list with a reason — add app-data functions there, never user-file ones.

## Database Migrations (IMPORTANT)
- Migration files: `backend/internal/database/migrations/YYYY_MM_DD_HHMMSS_name.go`
- Registry: `backend/internal/database/migrations/registry.go` — defines `Migration` struct, `Register()`, `All()`
- Runner: `backend/internal/database/db.go` → `runMigrations()` — runs on every server start
- Tracking table: `migrations` (version TEXT PK, name TEXT, applied_at TEXT)
- Each migration file has `Up` and `Down` funcs; runs in a transaction
- **To create a new migration:** `./dev.sh migrate <name>` (or `go run ./cmd/migrate <name>` from `backend/` dir)
  - e.g. `go run ./cmd/migrate add_email_to_users`
  - Generates a file with the current timestamp + name, whose Up/Down return an error until written — the dev server migrates as soon as a file is saved, and an empty migration would be recorded as applied and never run again
- **Never edit or delete existing migration files** — only append new ones

## Frontend
- Stack: Vue3 + TypeScript + Pinia + Vue Router v5 + Tailwind v4
- API client: `frontend/src/utils/api.ts` (uses fetch; the session cookie goes along by itself). Whether the user is logged in is only known by asking the server (`auth.restoreSession()`), as scripts can't read the cookie. Any 401 goes to the `onSessionEnded` handler (set in `main.ts`): the app logs out and opens the login page with a way back — new request paths must call `noteStatus(status)` like the others. Leaving for the login page goes through `showLoginAfterLogout()` (`router/index.ts`), which navigates *before* clearing the user's state. Long-open responses (the watch stream) re-check the session with `sessionStillValid(c)`.
- Stores: `auth.ts` (login/logout, user), `files.ts` (current path + directory listing)
- Types: `frontend/src/types/index.ts`
- Paths in the user's folder (relative, `/`-separated, `''` = home): `parentOf`, `isWithin`, `relativeTo`, `breadcrumbTrail` in `utils/paths.ts` — don't slice paths by hand
- Dev proxy: Vite proxies `/api` → `http://localhost:8080`
- Live folder updates: `GET /files/watch` (SSE, `backend/internal/handlers/watch.go`) starts with a `snapshot` event (the full listing, read after the watcher is live), resends one whenever events may have been lost, and pings every 25 s. `stores/files.ts` loads folders from that snapshot, reconnects with backoff (and on silence, tab visible, `online`), so every reconnect resyncs. After writing a file, don't call `filesStore.refresh()` — the stream reports the change; `refresh()` reopens the stream.

## Project Structure Convention
- `components/ui/` — generic reusable (AppModal, AppSpinner, AppCheckbox)
- `components/layout/` — shell components (AppLayout, AppHeader)
- `components/files/` — file-specific (FileList, FileIcon, FileBreadcrumb, FilePreview)
- `components/archive/` — ArchiveContents
- `components/usage/` — the disk usage modal (DiskUsageModal, opened from Details, rendered by FilesView) and its parts (UsageList, UsageTreemap)
- `components/admin/` — UserFormModal
- `views/` — page-level (LoginView, FilesView, AdminView, SettingsView)

## UI Component Rules
- **AppCheckbox.vue** — always use for checkboxes; supports `v-model`, `indeterminate` prop; never use native `<input type="checkbox">`
- **SecretInput.vue** — ALWAYS use for passwords that aren't the user's account password (archive passwords); NEVER a native `type="password"` there — password managers autofill the login into those and offer to save what's typed. Supports `v-model`; other attributes fall through. Account password fields stay `type="password"` with the right `autocomplete` (`current-password` / `new-password`).
- **Archive passwords** — asked for only when needed. The server answers a missing or wrong one with 400 `password_required` (`errPasswordRequired` + `respondArchiveError` in `archive.go`), which `api.ts` throws as `PasswordRequiredError`; listings mark encrypted files (`encrypted`). `useArchiveExtract()` returns `passwordPrompt` (`usePasswordPrompt`, one dialog per caller): render `<PasswordDialog :prompt="passwordPrompt" … />` where extraction starts, inside the modal that starts it.
- **AppSelect.vue** — ALWAYS use for dropdowns/selects; NEVER use native `<select>`. Supports `v-model`, `options: { label, value }[]`, `disabled`. Handles string and number values correctly. Usage: `<AppSelect v-model="val" :options="[{ label: 'Foo', value: 'foo' }]" />`
- **AppAccordion.vue + AppAccordionItem.vue** — use for collapsible settings sections; only one item open at a time. Wrap items in `<AppAccordion>`, each section in `<AppAccordionItem title="…">`. Uses CSS grid-template-rows transition for smooth animation.
- **AppModal.vue** — for larger complex modals: header with the close ✕ first (on the left — the preview's top bar too), title, footer slot; sizes sm/md/lg/xl/2xl (below `sm` it fills the screen); a `#title` slot replaces how the title is shown (the archive modal puts its breadcrumb there; still pass `title`, it names the modal for screen readers). Escape closes it by default (`close-on-escape="false"` only where Escape must mean something else; anything inside that uses Escape calls `preventDefault()` — `useEscapeToClose`, only the dialog on top closes); `dirty` when closing would lose what was typed — the ✕, Escape and a click outside then ask "Discard changes?" (Cancel in the footer doesn't); `fixed-height` (90vh whatever the content). Dialogs opened from inside a modal go inside its slot, so they count as on top of it.
- **Keyboard focus in modals** — `AppModal` and `AppDialog` use `useModalFocus`: focus moves in on open (to the element with `autofocus`, else the card), Tab stays inside the one on top, and focus returns to the opener on close. Mark the field to type in with `autofocus`.
- **PresetPill.vue** (`components/files/`) — full-width preset list row with left accent border when active, inline delete confirm; used in AdvancedRenamer presets popover. Props: `name`, `template`, `active` (blue left border + bg when current template matches). Emits: `select`, `delete`.
- **AppIcon.vue** — ALWAYS use for icons; NEVER write inline SVGs. Registry: `frontend/src/utils/icons.ts`. Usage: `<AppIcon name="trash" class="size-4" />`. Optional `stroke-width` prop. To add a new icon, add it to `icons.ts` first.
- **useToast** — ALWAYS use for user-facing messages (success, error, info). NEVER use `alert()`. Import: `import { useToast } from '@/composables/useToast'`. Usage: `const { show: showToast } = useToast()` then `showToast('message', 'success' | 'error' | 'info')`. `AppToast` is mounted once in `App.vue` (so messages show on the login page too) — no need to add it per-view.
- **Scrollbars** — themed once for the whole app in `src/style.css` (thin, translucent zinc thumb, no track); don't style them per component.
- **AppLoading.vue** — ALWAYS use for an area that's loading (not a small spinner alone): a bright spinner, `label` saying what's happening, and a slot for a hint, progress or Cancel. Keep `AppSpinner size="sm"` for inside buttons.
- **AppButton.vue** — ALWAYS use for modal/dialog buttons; NEVER write raw `<button>` elements with inline Tailwind classes in modals. Props: `variant` (`'primary'` | `'destructive'` | `'secondary'` | `'ghost'`, default ghost), `disabled`, `type`. Usage: `<AppButton variant="primary" @click="confirm">Save</AppButton>`. `disabled` automatically adds `opacity-50 cursor-not-allowed`.

## Editable File Types Convention (IMPORTANT)
File types with their own editor (`.todo` → `TodoViewer`, `.kanban` → `KanbanViewer`) follow one pattern — reuse it for new ones:
- **Load + auto-save:** `useEditableFile(() => props.entry, { state, parse, serialize, onLoaded?, carryOverIds? })` from `frontend/src/composables/useFileContent.ts` — parses the file into `state`, auto-saves the serialized state 800 ms after each change. Read-only loading (e.g. `TextViewer`) uses `useFileContent` from the same file.
- **Simultaneous editing:** saves are versioned — `GET /files/content` returns the version as `ETag`, `PUT /files/content` with `If-Match` answers 412 (`EditConflictError` in `api.ts`) with the current content when the file changed. `useEditableFile` merges both that and changes reported by the watch stream into the editor with `mergeText()` (`utils/merge.ts`, local version wins on overlap); `TextViewer` merges on save and falls back to conflict markers. Pass `carryOverIds` (built on `adoptIds()`) so re-parsed entries keep their ids and open dialogs/focus survive a merge. Never write editor content without the base version.
- **Loading / error / too-large UI:** wrap the editor in `<EditableFileState>` (`components/files/`).
- **Registration:** `isXFile()` + category in `utils/formatters.ts` (classified before the mime checks), the view's name in `customViewName()` (enables the preview header's "View as text" toggle and the "Open as text" file op), `FileCategory` in `types/index.ts`, icon in `icons.ts`, viewer in `FilePreview.vue` `viewerFor()`, and an entry in `newFileTypes` in `FilesView.vue` — the single list behind the toolbar, mobile menu and context-menu "New …" items (they create the file with `fileops.create`, which never overwrites an existing one). Backend `mimeByExt` maps the extension to `text/plain`.
- **Checklists:** `useChecklistEditor` (`composables/`) + `TodoRow.vue` for any Enter/Backspace-driven checkbox list.
- **Drag and drop:** `vue-draggable-plus` with `:force-fallback="true"`, `ghost-class="drag-ghost"`, `fallback-class="drag-fallback"` (global styles in `src/style.css`).
  - **Internal hook (check on every `vue-draggable-plus` upgrade):** `dragOverNearestList()` (`utils/sortable.ts`), which lets kanban cards drop anywhere above or below a column, calls SortableJS's private `_onDragOver` on the instance found by `sortableOf()` — the library has no public way to hand it a drag. `sortable.spec.ts` fails if the hook is gone, but not if it behaves differently: after upgrading, drag a card into the empty space under a short column and over a column's title by hand.
- **Markdown:** render with `renderMarkdown(source, entryPath)` from `utils/markdown.ts` inside a `.markdown-body` element. It returns sanitised HTML (DOMPurify), safe for `v-html`. Never put any other file content into `v-html`; HTML built as a string escapes its text with `escapeHtml()` from `utils/html.ts`.
- **Horizontal gestures:** mark regions that scroll/drag sideways with `data-no-carousel-swipe` so the preview carousel doesn't treat them as prev/next swipes. Don't `.stop` touch events instead — SortableJS listens on `document`.
- **Escape inside a viewer:** if an Escape handler removes the focused element, use `@keydown.escape.stop`, otherwise the preview's window listener sees focus on `<body>` and closes the preview.

## Debounce Convention
Always use `debounce()` from `frontend/src/utils/debounce.ts` — never write raw `clearTimeout`/`setTimeout` pairs. Returns a debounced function with a `.cancel()` method; call `.cancel()` in `onUnmounted` / watch cleanup to avoid stale timers. Usage: `const debouncedFn = debounce(fn, delayMs)`.

## Header Center Slot Convention (IMPORTANT)
`AppHeader` is a layout shell — it must never import or reference view-specific components directly. It renders a generic center slot via `layout.headerCenter: shallowRef<Component | null>` from the layout store.

Views that need to render content in the header call `useHeaderCenter(name, render)` (`composables/useHeaderCenter.ts`) with a render function that closes over their own reactive state; it sets the component while the view is mounted and clears it on unmount. AppHeader renders it blindly with `<component :is="layout.headerCenter">`.

Used by `FilesView` to render `FileBreadcrumb` in place of the logo. `FileBreadcrumb` also works elsewhere (the disk usage modal): `root` makes the trail start at a given folder, `hrefFor` sets the crumbs' links.

## File Operations Convention (IMPORTANT)
When adding a new file operation, add it ONLY to `buildFileOps()` in `frontend/src/composables/useFileOps.ts` — it is the single source of truth. The context menu (`contextMenuItems`) and toolbar (`FileOpsBar` via `fileOps` computed) both derive from it automatically. Each op is `{ key, label, icon, danger?, visible?, disabled?, action }`. Use `visible` for conditional ops (e.g. single-selection only).

`FilesView.vue` uses `useFileOps()` to get `clipboard`, `fileOps`, `contextMenuItems`, `onContextMenu`, `onPaste`, and `performMove`. New action callbacks (e.g. opening a dialog) must be passed via `actionCallbacks` when calling `useFileOps`.

## URL State Convention (IMPORTANT)
ALL browsing state must be reflected in the URL so page refresh restores it:
- `?path=` — current folder (synced via `navigate()` + `watch(route.query.path)`)
- `?preview=` — open preview file path
- `?q=` — search query
- `?view=text` — current preview shown as raw text instead of its custom view (`.todo`, `.kanban`)
- `?usage=` — disk usage modal open on that folder (empty = home folder), with `?usage_at=` the folder looked at inside it, `?usage_tab=largest`. Read with `usageStateFrom(query)`, written with `withUsageState(query, state | null)` (`utils/diskUsage.ts`), which keep the browser's own keys; `routeQuery()` in FilesView keeps the modal's.
- `?archive=` — archive modal open on that archive, with `?archive_at=` the folder looked at inside it. Read with `archiveStateFrom(query)`, written with `withArchiveState(query, state | null)` (`utils/archiveTree.ts`); both modals build on `queryString` / `withKeys` (`utils/query.ts`).

Any new state that should survive refresh MUST be added to the URL and synced bidirectionally with `watch(route.query.xxx)` + `watch(stateRef)`.

## Modal Design Rules (small dialogs: confirmations, a name to type)
- **AppDialog.vue** (`components/ui/`) — ALWAYS use for these; NEVER write the overlay + card by hand. `<AppDialog :open="showIt" @close="showIt = false">…</AppDialog>`; `width` prop for wider cards (default `w-72` for simple confirm/input; `w-80`/`max-w-sm` for lists or more content). It fades in, closes on a click outside and on Escape, and is marked `aria-modal`, so the preview and other shortcuts behind it ignore the keyboard (`isModalOpen()` in `utils/keyboard.ts` — check it in any new window-wide key handler).
- Title: `<h3 class="text-sm font-semibold text-zinc-100 mb-3">`
- Body text/description: `<p class="text-xs text-zinc-400 mb-3">`
- Text input: `w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors`
- Key handlers on inputs: `@keydown.enter` to confirm (AppDialog handles Escape)
- Buttons: ALWAYS in `<template #actions>` (right-aligned, 24px below the content, 8px apart); never a button row of your own — `AppDialog.spec.ts` fails on one. Don't end the content with a bottom margin to push them down.
- Spacing (Tailwind's 4px steps): a gap between groups is at least three times the gaps inside them — only slightly bigger (8 then 12 or 16) reads as one list. A label sits 4px from its field (`mb-1`) and fields 12px apart (`mb-3`); rows of a list 8px apart (`space-y-2`) and the next group 24px; the title 12px above the content (`mb-3`); the actions 24px below it (the slot does that).

## File Conflict Resolution Convention
When a file operation (move, copy, rename, advanced rename, upload, extract, compress, new file) would overwrite an existing file, the backend detects the conflict and returns HTTP 409 with `{ conflicts: string[] }` (list of conflicting filenames). The server always enforces this itself — never rely on a check in the app, the folder may have changed since it was listed. Nothing is written before the 409. The frontend then presents the user with a resolution modal offering:
- **Replace** — overwrite the existing file(s)
- **Keep all** — rename the incoming file(s) by appending a suffix (e.g. `file (1).txt`, `file (2).txt`) until no conflict remains
- **Cancel** — abort the operation entirely

The resolution choice is sent back to the backend as `conflict_mode: 'replace' | 'rename'` on the retry request (type `ConflictMode` in `api.ts`).

Items of the same batch never replace each other (two `readme.md` moved into one folder, two photos renamed to the same name): the later one always gets ` (n)`, whatever the mode — Replace only applies to what was already there. Backend helpers in `fileops.go`: `findNameConflicts` (409 list), `batchTargetName` (names for a batch), `resolveDestConflict` (a single destination), `moveEntry`; extraction stages in a temp folder and moves the result with `mergeInto` / `mergeConflicts` (`archive.go`), so a failed extract never touches existing files.

### Unified conflict dialog system (IMPORTANT)
All conflict flows use a single shared composable + component — do NOT create per-operation conflict dialogs:
- Composable: `frontend/src/composables/useConflictDialog.ts` — `prompt(title, names): Promise<'replace' | 'rename' | null>`
- Component: `frontend/src/components/ui/ConflictDialog.vue` — mounted once globally in `FilesView.vue` via `<ConflictDialog />`
- Usage: `const { runWithConflictPrompt } = useConflictDialog()` then `const result = await runWithConflictPrompt('Title', (conflictMode) => fileops.x(..., conflictMode))` — runs with `'error'`, asks on a conflict and runs again with the choice; resolves `null` when cancelled. Don't write the catch-ask-retry by hand. `prompt(title, names)` is there for asking before an operation (upload checks the listing first so a large upload isn't sent just to be refused).
- Archive extraction: `useArchiveExtract().extractArchive(path, password?)` (used by FilesView and the archive viewer).
- Backend `conflict_mode` parameter: `'replace'` maps to replace, `'rename'` maps to keep-all/rename

## External Tool Dependencies (must be in Dockerfile `runtime-base` stage)
**Running media tools (IMPORTANT):** ffmpeg, ffprobe and ImageMagick decide a file's format from its contents, and some formats they accept read other files or reach the network — a crafted file could make a thumbnail/preview read another user's files or an internal service (S7). Run them only through `internal/media`: `media.FFmpeg(ctx, input, inputOptions, outputArgs...)`, `media.FFprobe(path, opts...)` / `media.FFprobeContext(ctx, …)`, `media.MagickConvert(ctx, args...)` and `media.MagickIdentify(ctx, args...)`; `media.Command(ctx, …)` for other tools (it stops waiting shortly after ctx ends). Never call `exec.Command("ffmpeg"/"ffprobe"/"convert"/"identify", …)` yourself — `media`'s `TestMediaToolsOnlyRunHere` fails the build if anything outside the package does. Pass tools the checked real location from `fsys.RealPath(name)`, never a raw request path.

Any new CLI tool used by the backend must be added to the `apk add` line in the `runtime-base` stage of `backend/Dockerfile` (Alpine 3.24) — both the production runtime and the dev container build on it — and to the tool check right after it, which fails the build if a tool is missing or broken. Current tools:
- `ffmpeg` — video thumbnails + HLS transcoding
- `p7zip` — 7z/RAR archive support
- `rsvg-convert` (Alpine package `rsvg-convert`) — SVG rasterisation for thumbnails
- `imagemagick` + `imagemagick-heic` — thumbnails for AVIF, HEIC, HEIF, JXL, ICO, PSD, TGA, XCF, HDR, EXR, DDS
- `poppler-utils` — PDF utilities (e.g. `pdftoppm` for PDF thumbnails)
- `unrar` — RAR extraction; copied from `linuxserver/unrar:7.2.4` image as `/usr/bin/unrar-alpine → /usr/bin/unrar` (not an apk package)
- `ca-certificates` — TLS root certificates
- `tzdata` — timezone data

