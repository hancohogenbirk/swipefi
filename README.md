# SwipeFi

Self-hosted music player with a Tinder-like swipe interface for curating your collection. Plays bitperfect via DLNA/UPnP to any compatible renderer. Runs as a Docker container on a Synology NAS.

**Swipe right** to keep a track. **Swipe left** to move it to a `to_delete` folder. That's it.

## Features

- Bitperfect DLNA playback to any UPnP/DLNA renderer (WiiM, TV, Sonos, etc.)
- Tinder-like swipe UI — keep or discard tracks
- Spotify-style bottom tab navigation (Folders, Now Playing, Settings)
- Mini-player bar showing current track when browsing folders
- Folder browser with one-tap playback
- Reorderable queue with drag-to-reorder and skip-to
- Play count tracking (counts after 60 seconds of listening)
- Live play count updates on the Now Playing screen via WebSocket
- Audio format info on the now-playing card (sample rate, bit depth, bitrate) for FLAC files
- Spotify-style full-width progress bar with elapsed/remaining times below
- Sort by play count or date added
- Cover art display (embedded, MusicBrainz/Cover Art Archive fallback)
- Deletion management — restore or permanently delete rejected files from Settings
- Empty folder cleanup after permanent deletion (walks up directory tree)
- Cached cover art cleanup on permanent delete
- External device takeover detection (transitions to idle when another app takes over)
- Automatic device disconnection detection (returns to setup screen when device becomes unreachable)
- Reconnect state recovery (picks up the playing track and rebuilds the queue on reconnect)
- Browser back button support with in-tab navigation
- Loading indicator while DLNA renderer buffers a new track
- Graceful track transitions (stops current before starting next)
- DLNA connection retry on transient failures
- Fast library scanning (single-pass, optimized for network shares)
- Automatic partial rescan after rejecting tracks
- Scan cancellation when music directory changes
- Play counts and deletion state preserved when switching music directories
- Configurable music directory via in-app settings
- Single Docker container, single binary
- Auto-updates via Watchtower

## Tech Stack

- **Backend**: Go (chi, goupnp, modernc.org/sqlite, gorilla/websocket)
- **Frontend**: Svelte 5 + TypeScript + Vite (client-side SPA, embedded in Go binary)
- **Icons**: [Lucide](https://lucide.dev/) (via lucide-svelte)
- **Database**: SQLite
- **CI/CD**: GitHub Actions → ghcr.io → Watchtower

## Deploy on Synology NAS

### Prerequisites

- Synology NAS with DSM 7.2+
- **Container Manager** installed (Package Center → search "Container Manager")

### Step 1 — Create folders

In **File Station**, navigate to the `docker` shared folder:

```
docker/
├── swipefi/
│   └── data/
└── watchtower/
```

### Step 2 — Create the SwipeFi project

1. Open **Container Manager** → **Project** → **Create**
2. **Project name**: `swipefi`
3. **Path**: `/docker/swipefi`
4. **Source**: "Create docker-compose.yml", paste:

```yaml
services:
  swipefi:
    image: ghcr.io/hancohogenbirk/swipefi:latest
    container_name: swipefi
    pull_policy: always
    network_mode: host
    volumes:
      - /volume1/Audio:/audio
      - /volume1/docker/swipefi/data:/data
    environment:
      - SWIPEFI_PORT=8080
      - SWIPEFI_DATA_DIR=/data
    restart: unless-stopped
```

5. Click **Next** → skip Web Portal → **Done**

> Adjust `/volume1/Audio` to match your audio shared folder name.

### Step 3 — Create Watchtower (auto-updates)

1. **Container Manager** → **Project** → **Create**
2. **Project name**: `watchtower`
3. **Path**: `/docker/watchtower`
4. **Source**: "Create docker-compose.yml", paste:

```yaml
services:
  watchtower:
    image: nickfedor/watchtower:latest
    container_name: watchtower
    environment:
      - TZ=Europe/Brussels
      - WATCHTOWER_CLEANUP=true
      - WATCHTOWER_SCHEDULE=0 0 3 * * *
      - WATCHTOWER_INCLUDE_STOPPED=true
      - WATCHTOWER_REVIVE_STOPPED=false
      - DOCKER_API_VERSION=1.43
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    command: swipefi
    restart: unless-stopped
```

5. Click **Next** → **Done**

### Step 4 — Configure

1. Open `http://your-nas:8080`
2. Select your DLNA renderer (auto-discovered)
3. Go to the **Settings** tab → navigate to your music folder → tap the green button
4. Switch to the **Folders** tab → browse → play → swipe

## Usage

### Navigation

The app uses a bottom tab bar with three tabs:

- **Folders** — browse your music library, tap play on any folder
- **Now Playing** — swipe card interface, transport controls, queue access
- **Settings** — music directory picker, deletion management

### Swiping

On the Now Playing screen:
- **Swipe right** — keep the track, advance to next
- **Swipe left** — move track to `to_delete` folder, advance to next

### Deletion Management

Rejected files are moved to `<music_dir>/to_delete/`. To manage them:

1. Go to **Settings** tab → **Marked for Deletion**
2. Select files with checkboxes (or "Select All")
3. **Restore** — moves files back to their original location
4. **Delete Forever** — permanently removes files, cleans up empty folders and cached art

### Back Button

The browser back button navigates within the current tab (e.g., subfolder → parent folder, queue → now playing). At a tab root, it shows a "Leave SwipeFi?" confirmation.

## Local Development

### Prerequisites

- Go 1.26+
- Node.js 22+

### Run

```bash
# Terminal 1 — backend
SWIPEFI_MUSIC_DIR=/path/to/music SWIPEFI_DATA_DIR=./data go run ./cmd/swipefi

# Terminal 2 — frontend (with hot reload)
cd web && npm install && npm run dev
```

Open `http://localhost:5173`. The Vite dev server proxies API calls to the Go backend on port 8080.

### Build single binary

```bash
cd web && npm run build && cd ..
go build -ldflags="-s -w" -o swipefi ./cmd/swipefi
```

The binary embeds the frontend and serves everything on port 8080.

### Build Docker image

```bash
docker build -t swipefi .
```

The multi-stage Dockerfile builds the frontend, compiles a stripped Go binary, and compresses it with UPX for a minimal image.

### Run tests

```bash
# Go
go vet ./...
go test ./...

# Frontend type check
cd web && npx svelte-check

# E2E (requires backend running with music dir)
cd web && npx playwright test
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SWIPEFI_PORT` | `8080` | HTTP server port |
| `SWIPEFI_MUSIC_DIR` | *(none)* | Music directory (overrides in-app setting) |
| `SWIPEFI_DATA_DIR` | `./data` | SQLite database and art cache location |

If `SWIPEFI_MUSIC_DIR` is not set, the app prompts you to pick a directory in the Settings UI. The choice is saved to SQLite and persists across restarts.

Rejected files are always moved to `<music_dir>/to_delete/`.

## Architecture

```
Phone/Browser (Svelte SPA)
     ↕ REST + WebSocket
Go Backend (chi router)
     ↕ UPnP AV commands
DLNA Renderer ← pulls audio from Go's HTTP file server (bitperfect)
```

The backend acts as a UPnP control point — it discovers DLNA renderers on the network, sends playback commands, and serves raw audio files. No transcoding, no resampling. Works with any UPnP/DLNA compatible renderer.
