# SwipeFi

Self-hosted music player with Tinder-like swipe UI for curating your collection. Plays bitperfect via DLNA/UPnP to a WiiM renderer. Runs as a Docker container on a Synology NAS.

## Tech Stack

- **Backend**: Go (chi router, goupnp, modernc.org/sqlite, dhowden/tag, gorilla/websocket)
- **Frontend**: Svelte + TypeScript + Vite (client-side SPA, no SvelteKit)
- **Database**: SQLite (pure Go, no CGo)
- **Deployment**: Docker with host networking on Synology

## Architecture

- Go binary serves REST API, WebSocket, raw audio streaming, and embedded Svelte static files
- Backend acts as UPnP control point — discovers WiiM, sends playback commands
- WiiM pulls audio directly from backend's HTTP file server (bitperfect, no transcoding)
- Frontend is purely a control UI — it never touches audio

## Project Layout

- `cmd/swipefi/` — entry point
- `internal/api/` — HTTP handlers, WebSocket
- `internal/dlna/` — UPnP discovery and AVTransport control
- `internal/library/` — filesystem scanning, metadata extraction
- `internal/player/` — playback state machine, queue management
- `internal/fileserver/` — raw audio HTTP serving with range requests
- `internal/store/` — SQLite database, queries
- `web/` — Svelte frontend (built to static files, embedded in Go binary)

## Go Conventions

- Use `internal/` for all application packages — nothing is exported
- Errors: return `error`, don't panic. Wrap with `fmt.Errorf("context: %w", err)`
- Logging: use `log/slog` (structured logging)
- Context: pass `context.Context` as first parameter where appropriate
- No CGo — use `modernc.org/sqlite` for SQLite

## Frontend Conventions

- Plain Svelte + Vite (NOT SvelteKit)
- TypeScript strict mode
- Svelte stores for shared state (player state, library)
- Simple client-side router (`svelte-spa-router`)
- Mobile-first responsive design

## Git

Follow rules in `../git.md` — rebase only, no merge commits, short one-liner commit messages.
