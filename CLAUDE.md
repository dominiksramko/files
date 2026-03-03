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
- **AppModal.vue** — for larger complex modals with header X-button, title, footer slot; sizes sm/md/lg/xl

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

## External Tool Dependencies (must be in Dockerfile runtime stage)
Any new CLI tool used by the backend must be added to the `apk add` line in `backend/Dockerfile` runtime stage (Alpine). Current tools:
- `ffmpeg` — video thumbnails + HLS transcoding
- `p7zip` — 7z/RAR archive support
- `resvg` — SVG rasterisation for thumbnails

## Dev Commands
```bash
# Backend
cd backend && go run ./cmd/server

# Frontend
cd frontend && npm run dev

# Docker
docker compose up --build
```
