# SwipeFi

Self-hosted music player with a Tinder-like swipe interface for curating your collection. Plays bitperfect via DLNA/UPnP to a DLNA renderer renderer. Runs as a Docker container on a Synology NAS.

**Swipe right** to keep a track. **Swipe left** to move it to a `to_delete` folder. That's it.

## Features

- Bitperfect DLNA playback to any UPnP/DLNA renderer (WiiM, TV, Sonos, etc.)
- Tinder-like swipe UI — keep or discard tracks
- Folder browser with one-tap playback
- Play count tracking (counts after 60 seconds of listening)
- Sort by play count or date added
- Configurable music directory via in-app settings
- Single Docker container, single binary
- Auto-updates via Watchtower

## Tech Stack

- **Backend**: Go (chi, goupnp, modernc.org/sqlite, gorilla/websocket)
- **Frontend**: Svelte 5 + TypeScript + Vite (client-side SPA, embedded in Go binary)
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
2. **Settings** screen → navigate to `/audio/Lossless/FLAC` (or your music path) → tap the green button
3. Your DLNA renderer is auto-discovered → select it → browse folders → play → swipe

### Updating

Watchtower checks for new images at 3 AM daily. To force an immediate update:

**Container Manager → Project → `swipefi` → Action → Build**

(`pull_policy: always` ensures it pulls the latest image.)

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
go build -o swipefi ./cmd/swipefi
```

The binary embeds the frontend and serves everything on port 8080.

### Run tests

```bash
# Go
go vet ./...
go test ./...

# Frontend type check
cd web && npx svelte-check

# E2E (requires backend running with test music dir)
cd web && npx playwright test
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SWIPEFI_PORT` | `8080` | HTTP server port |
| `SWIPEFI_MUSIC_DIR` | *(none)* | Music directory (overrides in-app setting) |
| `SWIPEFI_DELETE_DIR` | `<music_dir>/to_delete` | Where rejected files are moved |
| `SWIPEFI_DATA_DIR` | `./data` | SQLite database location |

If `SWIPEFI_MUSIC_DIR` is not set, the app prompts you to pick a directory in the Settings UI. The choice is saved to SQLite and persists across restarts.

## Architecture

```
Phone/Browser (Svelte SPA)
     ↕ REST + WebSocket
Go Backend (chi router)
     ↕ UPnP AV commands
DLNA Renderer ← pulls audio from Go's HTTP file server (bitperfect)
```

The backend acts as a UPnP control point — it discovers DLNA renderers on the network, sends playback commands, and serves raw audio files. No transcoding, no resampling. Works with any UPnP/DLNA compatible renderer.
