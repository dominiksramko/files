# Files App — Project Structure

## Feature Checklist

### Authentication
- [x] Login page (username + password)
- [x] JWT-based session (stored in localStorage)
- [x] Route guards (redirect to login if unauthenticated)
- [x] Change own password
- [x] Default admin account (admin:admin on first run)

### File Browser
- [x] List directory contents (files + folders)
- [x] Navigate into folders (breadcrumb)
- [x] Download files
- [x] File type icons
- [x] Sort by name / size / date

### Archive Support
- [x] Preview archive contents (list files inside) — ZIP, TAR, RAR, 7Z
- [x] Preview password-protected archives
- [x] Extract archive (auto-detect single root folder)
- [x] Extract with password

### Media Preview
- [x] Images (inline)
- [x] Videos (inline player)
- [x] Audio (inline player)
- [ ] Office files (Word, Excel, PowerPoint) — planned for future

### Admin Panel
- [x] List all users
- [x] Create user (username, password, root folder path)
- [x] Edit user (change folder path, reset password)
- [x] Delete user
- [x] Toggle admin flag

### User Settings
- [x] Change own password

---

## Directory Structure

```
files/
├── STRUCTURE.md                   ← this file
├── docker-compose.yml
├── backend/
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── server/
│   │       └── main.go            ← entry point
│   └── internal/
│       ├── config/
│       │   └── config.go          ← env-based config
│       ├── database/
│       │   └── db.go              ← SQLite init + migrations
│       ├── models/
│       │   └── user.go            ← User model
│       ├── middleware/
│       │   └── auth.go            ← JWT auth middleware
│       └── handlers/
│           ├── auth.go            ← login, logout, password change
│           ├── files.go           ← list, download, stream
│           ├── archive.go         ← preview + extract archives
│           └── users.go           ← admin user CRUD
└── frontend/
    ├── Dockerfile
    ├── index.html
    ├── vite.config.ts
    ├── package.json
    └── src/
        ├── main.ts
        ├── App.vue
        ├── style.css
        ├── env.d.ts
        ├── types/
        │   └── index.ts           ← shared TypeScript types
        ├── utils/
        │   ├── api.ts             ← axios/fetch client with auth header
        │   └── formatters.ts      ← size, date, mime helpers
        ├── router/
        │   └── index.ts           ← routes + navigation guards
        ├── stores/
        │   ├── auth.ts            ← user session, login/logout
        │   └── files.ts           ← current path, file listing
        ├── components/
        │   ├── ui/
        │   │   ├── AppModal.vue       ← generic modal
        │   │   ├── AppButton.vue      ← styled button
        │   │   └── AppSpinner.vue     ← loading indicator
        │   ├── layout/
        │   │   ├── AppLayout.vue      ← authenticated shell (header + slot)
        │   │   └── AppHeader.vue      ← top bar with nav + user menu
        │   ├── files/
        │   │   ├── FileList.vue       ← table/grid of files
        │   │   ├── FileItem.vue       ← single row/card
        │   │   ├── FileBreadcrumb.vue ← path breadcrumb
        │   │   └── FilePreview.vue    ← image/video/audio preview
        │   ├── archive/
        │   │   └── ArchiveContents.vue ← list contents of archive
        │   └── admin/
        │       ├── UserTable.vue      ← list of users
        │       └── UserFormModal.vue  ← create/edit user form
        └── views/
            ├── LoginView.vue
            ├── FilesView.vue
            ├── AdminView.vue
            └── SettingsView.vue
```

---

## Architecture Decisions

| Concern | Choice | Reason |
|---|---|---|
| Auth | JWT (Bearer token) | Stateless, easy to implement |
| Token storage | localStorage | Simple; acceptable for LAN app |
| Archive handling | 7z CLI (p7zip) | Widest format support incl. password |
| Office preview | LibreOffice headless → HTML | Server-side conversion |
| DB | SQLite (modernc) | Zero-config, pure Go |
| API prefix | `/api/v1` | Clean separation from static files |
| CORS | Allowed in dev from `:5173` | Vite dev server proxy |

---

## API Routes

```
POST   /api/v1/auth/login           login → JWT
PUT    /api/v1/auth/password        change own password (authed)
GET    /api/v1/auth/me              current user info (authed)

GET    /api/v1/files/list           ?path=  → directory listing
GET    /api/v1/files/download       ?path=  → file download
GET    /api/v1/files/stream         ?path=  → range-aware stream
GET    /api/v1/files/archive        ?path=&password=  → list archive
POST   /api/v1/files/extract        { path, dest?, password? }

GET    /api/v1/users                admin: list users
POST   /api/v1/users                admin: create user
PUT    /api/v1/users/:id            admin: update user
DELETE /api/v1/users/:id            admin: delete user
```

---

## Environment Variables (backend)

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `JWT_SECRET` | `change-me` | JWT signing secret |
| `DB_PATH` | `./data/files.db` | SQLite database path |
| `USERS_ROOT` | `./data/users` | Base dir for user folders (dev) |
| `GIN_MODE` | `debug` | `debug` or `release` |
