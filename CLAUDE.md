# Project — files app

## What This Is
A self-hosted file browser with Vue3 frontend + Go backend.

## Key Paths
- `backend/` — Go + Gin + SQLite API server
- `frontend/` — Vue3 + Pinia + Tailwind SPA
- `docker-compose.yml` — production deployment

## Backend
- Entry: `backend/cmd/server/main.go`
- Config: `backend/internal/config/config.go` (env vars: PORT, JWT_SECRET, DB_PATH, USERS_ROOT)
- Auth middleware: `backend/internal/middleware/auth.go` — accepts JWT from `Authorization: Bearer` header OR `?token=` query param
- Handlers: `backend/internal/handlers/{auth,files,users,archive}.go`
- DB: SQLite via `modernc.org/sqlite` (pure Go, no CGO); auto-seeds admin:admin on first run
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
- Current migrations: `2026_02_26_000001_create_users_table`, `2026_02_26_000002_create_folder_prefs_table`, `2026_03_02_000001_add_display_name_to_users`

## Frontend
- Stack: Vue3 + TypeScript + Pinia + Vue Router v5 + Tailwind v4
- API client: `frontend/src/utils/api.ts` (uses fetch, attaches Bearer token)
- Stores: `auth.ts` (login/logout, user), `files.ts` (current path + directory listing)
- Types: `frontend/src/types/index.ts`
- Dev proxy: Vite proxies `/api` → `http://localhost:8080`

## Project Structure Convention
- `components/ui/` — generic reusable (AppModal, AppSpinner, AppCheckbox)
- `components/layout/` — shell components (AppLayout, AppHeader)
- `components/files/` — file-specific (FileList, FileIcon, FileBreadcrumb, FilePreview)
- `components/archive/` — ArchiveContents
- `components/admin/` — UserFormModal
- `views/` — page-level (LoginView, FilesView, AdminView, SettingsView)

## UI Component Rules
- **AppCheckbox.vue** — always use for checkboxes; supports `v-model`, `indeterminate` prop; never use native `<input type="checkbox">`
- **AppSelect.vue** — ALWAYS use for dropdowns/selects; NEVER use native `<select>`. Supports `v-model`, `options: { label, value }[]`, `disabled`. Handles string and number values correctly. Usage: `<AppSelect v-model="val" :options="[{ label: 'Foo', value: 'foo' }]" />`
- **AppAccordion.vue + AppAccordionItem.vue** — use for collapsible settings sections; only one item open at a time. Wrap items in `<AppAccordion>`, each section in `<AppAccordionItem title="…">`. Uses CSS grid-template-rows transition for smooth animation.
- **AppModal.vue** — for larger complex modals with header X-button, title, footer slot; sizes sm/md/lg/xl
- **AppIcon.vue** — ALWAYS use for icons; NEVER write inline SVGs. Registry: `frontend/src/utils/icons.ts`. Usage: `<AppIcon name="trash" class="size-4" />`. Optional `stroke-width` prop. To add a new icon, add it to `icons.ts` first.
- **useToast** — ALWAYS use for user-facing messages (success, error, info). NEVER use `alert()`. Import: `import { useToast } from '@/composables/useToast'`. Usage: `const { show: showToast } = useToast()` then `showToast('message', 'success' | 'error' | 'info')`. `AppToast` is mounted globally in `AppLayout` — no need to add it per-view.

## File Operations Convention (IMPORTANT)
When adding a new file operation, ALWAYS add it to BOTH:
1. The context menu — `contextMenuItems` computed in `FilesView.vue`
2. The FileOpsBar — `frontend/src/components/files/FileOpsBar.vue` (add emit + button), then wire the emit in the `<FileOpsBar>` usage in `FilesView.vue`

## Implemented Features
- JWT auth (login, guards, auto-restore on reload, password change, display name)
- File browser (list, navigate, sort, download)
- File preview: images (with checkerboard for transparent formats), video, audio, PDF (inline)
- Thumbnails: images, video (ffmpeg), SVG (resvg with checkerboard compositing)
- Archive preview + extraction: ZIP/TAR (pure Go) + 7z CLI fallback for RAR/7z/password-protected
- Admin panel: create/edit/delete users, toggle admin, display name
- Settings: change own password, display name
- Docker: multi-stage builds, nginx reverse proxy to backend

## URL State Convention (IMPORTANT)
ALL browsing state must be reflected in the URL so page refresh restores it:
- `?path=` — current folder (synced via `navigate()` + `watch(route.query.path)`)
- `?preview=` — open preview file path
- `?q=` — search query

Any new state that should survive refresh MUST be added to the URL and synced bidirectionally with `watch(route.query.xxx)` + `watch(stateRef)`.

## Modal Design Rules (FilesView.vue inline modals)
- Wrapper: `<Transition name="fade"><div v-if="..." class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" @click.self="close">`
- Card: `bg-zinc-900 border border-zinc-700 rounded-xl p-5 shadow-2xl`
- Width: `w-72` for simple confirm/input; `max-w-sm` for modals with lists or more content
- Title: `<h3 class="text-sm font-semibold text-zinc-100 mb-3">`
- Body text/description: `<p class="text-xs text-zinc-400 mb-3">`
- Text input: `w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors`
- Key handlers on inputs: `@keydown.enter` to confirm, `@keydown.escape` to close
- Footer buttons: `<div class="flex gap-2 mt-3 justify-end">`
  - Cancel: `px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800`
  - Primary: `px-3 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors`
  - Destructive: `px-3 py-1.5 text-sm bg-red-700 hover:bg-red-600 text-white rounded-lg transition-colors`
  - Secondary/neutral: `px-3 py-1.5 text-sm bg-zinc-700 hover:bg-zinc-600 text-white rounded-lg transition-colors`

## File Conflict Resolution Convention
When a file operation (move, copy, upload, extract, compress) would overwrite an existing file, the backend detects the conflict and returns HTTP 409 with `{ conflicts: string[] }` (list of conflicting filenames). The frontend then presents the user with a resolution modal offering:
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

## External Tool Dependencies (must be in Dockerfile runtime stage)
Any new CLI tool used by the backend must be added to the `apk add` line in `backend/Dockerfile` runtime stage (Alpine). Current tools:
- `ffmpeg` — video thumbnails + HLS transcoding
- `p7zip` — 7z/RAR archive support
- `rsvg-convert` (Alpine package) — SVG rasterisation for thumbnails
- `imagemagick` — thumbnails for AVIF, HEIC, HEIF, JXL, ICO, PSD, TGA, XCF, HDR, EXR, DDS

## Dev Commands
```bash
# Backend
cd backend && go run ./cmd/server

# Frontend
cd frontend && npm run dev

# Docker
docker compose up --build
```
