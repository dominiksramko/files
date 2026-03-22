# Files

A self-hosted file browser with a Vue 3 frontend and Go backend. Browse, preview, and manage your files from any browser.

## Features

### File Browser
- Navigate directories, sort by name/size/date
- Multi-select with checkbox or keyboard shortcuts
- Create folders and new text files
- Rename, move, copy, delete — single or batch
- Upload files with drag-and-drop or file picker
- Download files or bulk-download as ZIP
- Real-time file system watching (directory updates without refresh)
- Search within the current directory

### Previews
- **Images** — JPEG, PNG, GIF, WebP, BMP, TIFF, SVG, plus AVIF, HEIC, HEIF, JXL, ICO, PSD, TGA, XCF, HDR, EXR, DDS (converted server-side via ImageMagick)
- **Video** — MP4, WebM, MKV, AVI, MOV — with adaptive HLS transcoding, subtitle track selection, and screenshot capture at the current timestamp
- **Audio** — MP3, OGG, WAV, FLAC, AAC, M4A
- **PDF** — inline viewer with fallback for mobile
- **Text / Code** — plain text, Markdown, JSON, XML, HTML, CSS, JS, and more
- **Office** — DOC, DOCX, XLS, XLSX, PPT, PPTX, ODT, ODS, ODP (rendered via Microsoft Office Online — requires the server to be publicly reachable)
- **Archives** — browse contents of ZIP, TAR, TAR.GZ, TAR.BZ2, TAR.XZ, TAR.ZST, RAR, 7Z without extracting

### Thumbnails
Generated automatically and cached for: JPEG, PNG, GIF, WebP, BMP, TIFF (pure Go), SVG, PDF, video, and all ImageMagick-supported formats above.

### Archive Operations
- Extract ZIP, TAR, TAR.GZ, TAR.BZ2, TAR.XZ, TAR.ZST, RAR, 7Z (including password-protected)
- Compress selected files to ZIP or 7Z (with optional password)
- File conflict resolution: replace, keep both, or cancel

### Advanced Renamer
- Batch rename with a template language (counters, original name, extension, date, etc.)
- Live preview before applying
- Save and manage named presets
- Per-folder template memory

### Watch Progress
Video and audio resume from where you left off, per user.

### Subtitles
- Load external subtitle files alongside video
- Per-user style preferences (font, size, colour, position)

### Multi-user
- JWT-based authentication
- Each user has their own root directory (restricts access to their folder)
- Admin panel to create, edit, and delete users
- Per-user display names and passwords

---

## Deployment

### Quick start

1. Copy the example env file and fill in the required values:

```bash
cp .env.example .env
```

```env
# Required — generate with: openssl rand -hex 32
JWT_SECRET=

# Optional — admin account created on first run
# If ADMIN_PASSWORD is left empty, a random password is printed to the logs
ADMIN_USERNAME=admin
ADMIN_PASSWORD=
ADMIN_PATH=
```

2. Mount your files by adding volumes to `docker-compose.yml`:

```yaml
volumes:
  - db_data:/data
  - /your/files:/data/users/admin   # the admin's root folder
```

3. Start the container:

```bash
docker compose up -d
```

The app is available at `http://localhost:8080`.

### Reverse proxy

The app listens on port 8080 by default (override with `PORT` in `.env`). Put any reverse proxy in front of it — Caddy, nginx, Traefik, or none at all.

### Configuration

| Variable | Default | Description |
|---|---|---|
| `JWT_SECRET` | *(required)* | Signs auth tokens. Auto-generated (sessions lost on restart) if unset. |
| `PORT` | `8080` | Port the server listens on. |
| `DB_PATH` | `/data/files.db` | SQLite database path. |
| `USERS_ROOT` | `/data/users` | Parent directory for all user root folders. |
| `THUMB_CACHE_DIR` | `/data/thumbs` | Thumbnail cache directory. |
| `HLS_CACHE_DIR` | `/data/hls` | HLS transcoding cache directory. |
| `CONV_CACHE_DIR` | `/data/conv` | Office conversion cache directory. |
| `ADMIN_USERNAME` | `admin` | Initial admin username (first run only). |
| `ADMIN_PASSWORD` | *(random)* | Initial admin password (first run only). Printed to logs if unset. |
| `ADMIN_PATH` | *(empty)* | Initial admin root directory (first run only). Can be changed later. |

---

## Development

**Backend** (Go + Gin + SQLite):
```bash
cd backend && go run ./cmd/server
```

**Frontend** (Vue 3 + Vite):
```bash
cd frontend && npm install && npm run dev
```

Vite proxies `/api` to `http://localhost:8080`, so both servers need to be running.

**Docker** (full production build):
```bash
docker compose up --build
```
