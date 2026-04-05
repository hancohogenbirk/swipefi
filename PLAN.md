# SwipeFi — Architecture & Implementation Plan

## Overview

SwipeFi is a self-hosted music player with a Tinder-like swipe interface for curating your music collection. It runs as a Docker container on a Synology NAS, plays music **bitperfect** via DLNA/UPnP to a WiiM renderer, and lets you swipe right to keep or swipe left to discard tracks (moved to a `to_delete` folder).

## Tech Stack

| Layer | Choice | Why |
|-------|--------|-----|
| Backend | **Go** | Best UPnP library ecosystem (goupnp), goroutines for concurrency, static binary for Docker |
| Frontend | **Svelte + TypeScript + Vite** | Client-side SPA, compiles to static files embedded in Go binary, lightweight, great animations for swipe UX |
| Database | **SQLite** via `modernc.org/sqlite` | Pure Go (no CGo), single file, trivial backup, perfect for single-user |
| UPnP/DLNA | **huin/goupnp** | Pre-generated AVTransport/RenderingControl clients, SSDP discovery |
| Audio tags | **dhowden/tag** | FLAC, MP3, AAC, OGG metadata + album art extraction |
| Router | **go-chi/chi** | Lightweight, idiomatic, middleware-friendly |
| WebSocket | **gorilla/websocket** | Stable, widely used, pushes real-time playback state |
| Deployment | **Docker** on Synology | Host networking (required for UPnP multicast), single container |

## Architecture

```
┌─────────────────────────────────────┐
│  Phone/Tablet Browser               │
│  (Svelte SPA — swipe UI, controls)  │
└──────────┬──────────────────────────┘
           │ HTTP REST + WebSocket
           ▼
┌─────────────────────────────────────────────────────┐
│  SwipeFi (Go binary) — Docker on Synology           │
│  network_mode: host (required for UPnP multicast)   │
│                                                     │
│  ┌─────────────────────────────────────────────┐    │
│  │  HTTP Server (chi)                          │    │
│  │  ├── /api/folders      folder browsing      │    │
│  │  ├── /api/tracks       library + sorting    │    │
│  │  ├── /api/player/*     playback control     │    │
│  │  ├── /api/devices      DLNA renderer list   │    │
│  │  ├── /stream/{path}    raw audio serving    │    │
│  │  └── /ws               WebSocket state push │    │
│  └─────────────────────────────────────────────┘    │
│  ┌──────────────┐ ┌──────────┐ ┌───────────────┐   │
│  │ UPnP Control │ │ Library  │ │ SQLite        │   │
│  │ Point        │ │ Scanner  │ │ ├── tracks    │   │
│  │ (goupnp)     │ │ (tag)    │ │ └── playback  │   │
│  └──────┬───────┘ └──────────┘ └───────────────┘   │
│         │                                           │
│    /music (NAS volume, read/write)                  │
│    /data  (SQLite + config)                         │
└─────────┼───────────────────────────────────────────┘
          │ UPnP AV (SetAVTransportURI, Play, Stop, Seek...)
          ▼
    ┌───────────┐
    │   WiiM    │ ← pulls http://<synology-ip>:8080/stream/...
    │ (renderer)│   (bitperfect, raw FLAC/MP3 bytes)
    └───────────┘
```

## Screens

### Folder Navigation (home screen)

- Browse the music directory tree
- Each folder shows name and a **Play** button
- Tapping a folder navigates into it
- Tapping Play queues all songs in that folder + subfolders recursively
- Songs are queued in the currently selected sort order
- Sort selector: play count asc/desc, date added asc/desc
- Back button navigates up one level

### Now Playing (main screen)

- **Swipe card** (center): large card showing placeholder art, title, artist, album, play count
  - Swipe left → card flies off left (red tint), file moved to `to_delete/`, next track plays
  - Swipe right → card flies off right (green tint), next track plays, file stays
- **Progress bar**: seekable, shows elapsed time (left) and remaining time (right)
- **Transport controls**: Previous | -15s | Play/Pause | +15s | Next
- **Back button**: returns to folder navigation

## Database Schema

```sql
CREATE TABLE tracks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    path        TEXT UNIQUE NOT NULL,     -- relative to music root, e.g. "Jazz/Coltrane/track.flac"
    title       TEXT NOT NULL DEFAULT '', 
    artist      TEXT NOT NULL DEFAULT '',
    album       TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    format      TEXT NOT NULL DEFAULT '', -- "flac", "mp3", etc.
    play_count  INTEGER NOT NULL DEFAULT 0,
    added_at    INTEGER NOT NULL,         -- unix epoch, from file mtime on NAS
    last_played INTEGER,                  -- unix epoch, null if never played
    deleted     INTEGER NOT NULL DEFAULT 0, -- 1 if moved to to_delete
    created_at  INTEGER NOT NULL          -- unix epoch, when scanned into DB
);

CREATE INDEX idx_tracks_play_count ON tracks(play_count);
CREATE INDEX idx_tracks_added_at ON tracks(added_at);
CREATE INDEX idx_tracks_path ON tracks(path);
```

## API Endpoints

### Library

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/folders?path=` | List subfolders at path (default: root) |
| GET | `/api/tracks?folder=&sort=&order=` | List tracks in folder+subfolders, sorted |
| POST | `/api/library/scan` | Trigger a library rescan |

Sort values: `play_count`, `added_at`. Order: `asc`, `desc`.

### Player

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/player/play` | Start playing (body: `{folder, sort, order}`) |
| POST | `/api/player/pause` | Pause playback |
| POST | `/api/player/resume` | Resume playback |
| POST | `/api/player/next` | Skip to next track in queue |
| POST | `/api/player/prev` | Go to previous track in queue |
| POST | `/api/player/seek` | Seek to position (body: `{position_ms}`) |
| POST | `/api/player/reject` | Swipe left: move current track to `to_delete/`, skip to next |
| GET | `/api/player/state` | Get current player state (for initial load) |

### Devices

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/devices` | List discovered DLNA renderers |
| POST | `/api/devices/select` | Select renderer (body: `{device_id}`) |

### Streaming

| Method | Path | Description |
|--------|------|-------------|
| GET | `/stream/{path...}` | Serve raw audio file (range requests, no transcoding) |

### WebSocket

| Path | Description |
|------|-------------|
| `/ws` | Real-time player state push (JSON messages) |

WebSocket message:
```json
{
  "state": "playing",
  "track": {"id": 1, "title": "...", "artist": "...", "album": "...", "duration_ms": 240000, "play_count": 3},
  "position_ms": 45000,
  "queue_length": 42,
  "queue_position": 7
}
```

## Play Count Logic

- When the backend sends `Play` to the WiiM, start a server-side timer
- If paused, pause the timer. If resumed, resume it
- When accumulated play time reaches **60 seconds**, increment `play_count` and set `last_played`
- If the track is skipped or rejected before 60 seconds: no increment
- Play count is incremented **at most once per play session** (not per 60-second interval)

## File Management

- Swipe left (reject): `os.Rename()` moves the file from `/music/path/to/track.flac` to `/music/to_delete/path/to/track.flac`
- Directory structure is preserved under `to_delete/` for easy recovery
- The `to_delete/` subdirectories are created automatically
- Track is marked `deleted=1` in SQLite and removed from the current queue

## Docker Deployment

```yaml
services:
  swipefi:
    build: .
    network_mode: host    # REQUIRED for UPnP/SSDP multicast discovery
    volumes:
      - /volume1/music:/music          # NAS music share (read/write)
      - /volume1/docker/swipefi:/data  # SQLite + config persistence
    environment:
      - SWIPEFI_PORT=8080
      - SWIPEFI_MUSIC_DIR=/music
      - SWIPEFI_DELETE_DIR=/music/to_delete
      - SWIPEFI_DATA_DIR=/data
```

Dockerfile: multi-stage build
1. Stage 1: Node — build Svelte frontend (`npm run build`)
2. Stage 2: Go — build Go binary with embedded frontend (`go build`)
3. Stage 3: Scratch/Alpine — copy binary only, minimal image

## Implementation Phases

### Phase 1: Project Scaffold & Database
- [ ] Initialize Go module with dependencies
- [ ] SQLite setup: connection, schema creation, migrations
- [ ] Library scanner: recursive directory walk, metadata extraction (dhowden/tag)
- [ ] Populate tracks table from filesystem scan
- [ ] Scaffold Svelte + Vite frontend with basic shell
- [ ] Chi router with health check endpoint
- **Deliverable**: `go run` starts server, scans /music, stores tracks in SQLite

### Phase 2: REST API & Audio Streaming
- [ ] `GET /api/folders` — list subdirectories
- [ ] `GET /api/tracks` — list tracks with sorting (play_count, added_at) and filtering by folder
- [ ] `GET /stream/{path}` — raw audio file serving with Content-Type and Range headers
- [ ] Rescan endpoint
- **Deliverable**: Can browse folders, list sorted tracks, and stream audio via direct URL

### Phase 3: DLNA Discovery & Playback Control
- [ ] SSDP discovery: find UPnP MediaRenderers on LAN
- [ ] Auto-detect WiiM, allow manual selection
- [ ] AVTransport control: SetAVTransportURI, Play, Stop, Pause, Seek
- [ ] UPnP event subscription for transport state changes
- [ ] Player state machine: queue management, track progression, play count timer
- [ ] Player API endpoints (play, pause, resume, next, prev, seek, reject)
- [ ] WebSocket hub for real-time state broadcast
- **Deliverable**: Can play music on WiiM via API, tracks advance automatically

### Phase 4: Frontend — Folder Navigation
- [ ] Svelte SPA setup with client-side router
- [ ] Folder browser component: navigate directories, sort selector
- [ ] Play button: POST to /api/player/play with folder + sort params
- [ ] Navigate into Now Playing when playback starts
- **Deliverable**: Can browse folders in browser and start playback with one tap

### Phase 5: Frontend — Now Playing & Swipe
- [ ] SwipeCard component with Tinder-like gesture handling
- [ ] Swipe left animation (red) → reject, swipe right animation (green) → next
- [ ] Now Playing layout: card + progress bar + transport controls
- [ ] WebSocket integration for real-time state updates
- [ ] Transport controls: prev, -15s, play/pause, +15s, next
- [ ] Seekable progress bar with elapsed/remaining time
- [ ] Back navigation to folder browser
- **Deliverable**: Full swipe experience working end-to-end

### Phase 6: Docker & Deploy
- [ ] Multi-stage Dockerfile (Node build → Go build → minimal runtime)
- [ ] docker-compose.yml with host networking and volume mounts
- [ ] Go embed for frontend static files
- [ ] Test on Synology
- **Deliverable**: Single `docker compose up` deploys everything

## Folder Structure

```
swipefi/
├── cmd/
│   └── swipefi/
│       └── main.go              # Entry point, wire everything up
├── internal/
│   ├── api/
│   │   ├── router.go            # Chi router, middleware, static file serving
│   │   ├── tracks.go            # Track listing, sorting, folder browsing
│   │   ├── player.go            # Playback control endpoints
│   │   ├── devices.go           # DLNA device list/select endpoints
│   │   └── ws.go                # WebSocket hub, broadcast state
│   ├── dlna/
│   │   ├── discovery.go         # SSDP discovery, find renderers
│   │   ├── transport.go         # AVTransport SOAP actions
│   │   └── events.go            # UPnP event subscriptions
│   ├── library/
│   │   ├── scanner.go           # Recursive filesystem walk, index tracks
│   │   └── metadata.go          # Tag extraction via dhowden/tag
│   ├── player/
│   │   ├── player.go            # State machine, play count timer
│   │   └── queue.go             # Track queue, ordering, navigation
│   ├── fileserver/
│   │   └── fileserver.go        # Raw audio serving, range requests
│   └── store/
│       ├── db.go                # SQLite connection, schema, migrations
│       └── queries.go           # Track CRUD, sorted queries
├── web/
│   ├── src/
│   │   ├── App.svelte           # Root component, router
│   │   ├── main.ts              # Entry point
│   │   ├── lib/
│   │   │   ├── api/
│   │   │   │   └── client.ts    # Typed REST client
│   │   │   ├── stores/
│   │   │   │   ├── player.ts    # Player state (Svelte store)
│   │   │   │   └── library.ts   # Library/folder state
│   │   │   └── components/
│   │   │       ├── SwipeCard.svelte
│   │   │       ├── NowPlaying.svelte
│   │   │       ├── FolderNav.svelte
│   │   │       ├── TransportControls.svelte
│   │   │       └── ProgressBar.svelte
│   │   └── app.css              # Global styles
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   ├── svelte.config.js
│   └── vite.config.ts
├── api/
│   └── openapi.yaml             # API contract
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── Makefile
├── CLAUDE.md
├── PLAN.md
└── .gitignore
```
