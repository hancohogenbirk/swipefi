# Deletion & Undelete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add permanent deletion of rejected files, restore functionality, centralize to_delete folder, clean up empty folders and cached art.

**Architecture:** New store methods for listing/purging/restoring deleted tracks. New API endpoints for deleted track management. A shared folder cleanup utility. A new DeletedManager.svelte component in the Settings tab. Simplify delete_dir to always derive from music_dir.

**Tech Stack:** Go, SQLite, Svelte 5, TypeScript, lucide-svelte

---

### Task 1: Simplify delete_dir — remove separate config

**Files:**
- Modify: `cmd/swipefi/main.go`
- Modify: `internal/api/config.go`
- Modify: `internal/api/router.go`
- Modify: `internal/player/player.go`

- [ ] **Step 1: Update main.go — remove SWIPEFI_DELETE_DIR and delete_dir config**

In `cmd/swipefi/main.go`, replace lines 50-61:

```go
	// Determine music dir: env var overrides DB config
	musicDir := os.Getenv("SWIPEFI_MUSIC_DIR")
	if musicDir == "" {
		musicDir, _ = s.GetConfig("music_dir")
	}
	deleteDir := os.Getenv("SWIPEFI_DELETE_DIR")
	if deleteDir == "" {
		deleteDir, _ = s.GetConfig("delete_dir")
	}
	if musicDir != "" && deleteDir == "" {
		deleteDir = filepath.Join(musicDir, "to_delete")
	}
```

With:

```go
	// Determine music dir: env var overrides DB config
	musicDir := os.Getenv("SWIPEFI_MUSIC_DIR")
	if musicDir == "" {
		musicDir, _ = s.GetConfig("music_dir")
	}
	// delete_dir is always derived from music_dir
	var deleteDir string
	if musicDir != "" {
		deleteDir = filepath.Join(musicDir, "to_delete")
	}
```

- [ ] **Step 2: Update SetMusicDir in config.go — remove delete_dir DB write**

In `internal/api/config.go`, in `SetMusicDir`, remove this line:

```go
	a.store.SetConfig("delete_dir", deleteDir)
```

Keep the `deleteDir := filepath.Join(req.Path, "to_delete")` computation and the rest.

- [ ] **Step 3: Update GetAppConfig in config.go — derive delete_dir**

Replace the `GetAppConfig` method:

```go
func (a *API) GetAppConfig(w http.ResponseWriter, r *http.Request) {
	musicDir := a.scanner.MusicDir()
	var deleteDir string
	if musicDir != "" {
		deleteDir = filepath.Join(musicDir, "to_delete")
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"music_dir":  musicDir,
		"delete_dir": deleteDir,
	})
}
```

- [ ] **Step 4: Update onMusicDirChanged callback in main.go**

The callback signature `func(musicDir, deleteDir string)` can stay the same since SetMusicDir still computes and passes deleteDir. No change needed here.

- [ ] **Step 5: Verify build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add cmd/swipefi/main.go internal/api/config.go
git commit -m "simplify delete_dir: always derive from music_dir"
```

---

### Task 2: Add store methods for deleted track management

**Files:**
- Modify: `internal/store/queries.go`

- [ ] **Step 1: Add ListDeleted method**

```go
// ListDeleted returns all tracks marked as deleted.
func (s *Store) ListDeleted(ctx context.Context) ([]Track, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, path, title, artist, album, duration_ms, format, play_count, added_at, last_played, deleted
		FROM tracks WHERE deleted = 1 ORDER BY title ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list deleted: %w", err)
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		var lastPlayed sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.DurationMs,
			&t.Format, &t.PlayCount, &t.AddedAt, &lastPlayed, &t.Deleted); err != nil {
			return nil, fmt.Errorf("scan deleted track: %w", err)
		}
		if lastPlayed.Valid {
			t.LastPlayed = &lastPlayed.Int64
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}
```

- [ ] **Step 2: Add UnmarkDeleted method**

```go
// UnmarkDeleted sets deleted = 0 for the given track ID.
func (s *Store) UnmarkDeleted(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "UPDATE tracks SET deleted = 0 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("unmark deleted %d: %w", id, err)
	}
	return nil
}
```

- [ ] **Step 3: Add PurgeTrack method**

```go
// PurgeTrack permanently removes a track from the database.
func (s *Store) PurgeTrack(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM tracks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("purge track %d: %w", id, err)
	}
	return nil
}
```

- [ ] **Step 4: Add DeletedCount method**

```go
// DeletedCount returns the number of tracks marked as deleted.
func (s *Store) DeletedCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks WHERE deleted = 1").Scan(&count)
	return count, err
}
```

- [ ] **Step 5: Verify build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/store/queries.go
git commit -m "add store methods for deleted track management"
```

---

### Task 3: Add folder cleanup utility and cached art deletion

**Files:**
- Create: `internal/library/cleanup.go`
- Modify: `internal/api/art.go`

- [ ] **Step 1: Create cleanup.go with CleanupEmptyDirs**

```go
package library

import (
	"os"
	"path/filepath"
)

// CleanupEmptyDirs removes dir if empty, then walks up deleting empty parents.
// Stops at stopAt (never deletes stopAt itself).
func CleanupEmptyDirs(dir string, stopAt string) {
	dir = filepath.Clean(dir)
	stopAt = filepath.Clean(stopAt)

	for dir != stopAt && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}
```

- [ ] **Step 2: Add DeleteCachedArt to art.go**

In `internal/api/art.go`, add:

```go
// DeleteCachedArt removes all cached art files for the given track ID.
func DeleteCachedArt(cacheDir string, trackID int64) {
	for _, ext := range []string{".jpg", ".png", ".noart"} {
		path := filepath.Join(cacheDir, fmt.Sprintf("%d%s", trackID, ext))
		os.Remove(path)
	}
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/library/cleanup.go internal/api/art.go
git commit -m "add folder cleanup utility and cached art deletion"
```

---

### Task 4: Add deleted tracks API endpoints

**Files:**
- Create: `internal/api/deleted.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Create deleted.go with all three handlers**

```go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"swipefi/internal/library"
)

func (a *API) ListDeleted(w http.ResponseWriter, r *http.Request) {
	tracks, err := a.store.ListDeleted(r.Context())
	if err != nil {
		slog.Error("list deleted", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to list deleted tracks")
		return
	}
	if tracks == nil {
		tracks = []store.Track{}
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (a *API) RestoreDeleted(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	musicDir := a.scanner.MusicDir()
	if musicDir == "" {
		writeError(w, http.StatusBadRequest, "music directory not configured")
		return
	}
	deleteDir := filepath.Join(musicDir, "to_delete")

	restored := 0
	for _, id := range req.IDs {
		track, err := a.store.GetTrack(r.Context(), id)
		if err != nil {
			slog.Warn("restore: track not found", "id", id, "err", err)
			continue
		}

		src := filepath.Join(deleteDir, filepath.FromSlash(track.Path))
		dst := filepath.Join(musicDir, filepath.FromSlash(track.Path))

		// Create parent dirs at original location
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			slog.Error("restore: mkdir", "path", dst, "err", err)
			continue
		}

		// Move file back
		if err := os.Rename(src, dst); err != nil {
			slog.Error("restore: rename", "src", src, "dst", dst, "err", err)
			continue
		}

		// Unmark in database
		if err := a.store.UnmarkDeleted(r.Context(), id); err != nil {
			slog.Error("restore: unmark", "id", id, "err", err)
			continue
		}

		// Clean up empty dirs in to_delete
		library.CleanupEmptyDirs(filepath.Dir(src), deleteDir)

		slog.Info("restored track", "id", id, "path", track.Path)
		restored++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"restored": restored,
	})
}

func (a *API) PurgeDeleted(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	musicDir := a.scanner.MusicDir()
	if musicDir == "" {
		writeError(w, http.StatusBadRequest, "music directory not configured")
		return
	}
	deleteDir := filepath.Join(musicDir, "to_delete")

	dataDir := os.Getenv("SWIPEFI_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	cacheDir := filepath.Join(dataDir, "art")

	// If purging all, get the full list of deleted track IDs
	ids := req.IDs
	if req.All {
		tracks, err := a.store.ListDeleted(r.Context())
		if err != nil {
			slog.Error("purge: list deleted", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to list deleted tracks")
			return
		}
		ids = make([]int64, len(tracks))
		for i, t := range tracks {
			ids[i] = t.ID
		}
	}

	purged := 0
	for _, id := range ids {
		track, err := a.store.GetTrack(r.Context(), id)
		if err != nil {
			slog.Warn("purge: track not found", "id", id, "err", err)
			continue
		}

		// Delete the file from to_delete
		deletedFilePath := filepath.Join(deleteDir, filepath.FromSlash(track.Path))
		if err := os.Remove(deletedFilePath); err != nil && !os.IsNotExist(err) {
			slog.Error("purge: remove file", "path", deletedFilePath, "err", err)
			continue
		}

		// Delete cached art
		DeleteCachedArt(cacheDir, id)

		// Hard-delete from database
		if err := a.store.PurgeTrack(r.Context(), id); err != nil {
			slog.Error("purge: db delete", "id", id, "err", err)
			continue
		}

		// Clean up empty original source folder
		originalDir := filepath.Dir(filepath.Join(musicDir, filepath.FromSlash(track.Path)))
		library.CleanupEmptyDirs(originalDir, musicDir)

		// Clean up empty dirs in to_delete
		library.CleanupEmptyDirs(filepath.Dir(deletedFilePath), deleteDir)

		slog.Info("purged track", "id", id, "path", track.Path)
		purged++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"purged": purged,
	})
}
```

- [ ] **Step 2: Add the `store` import to deleted.go**

The file needs the store import for `store.Track{}`. Add to imports:

```go
import (
	...
	"swipefi/internal/store"
)
```

- [ ] **Step 3: Register routes in router.go**

In `internal/api/router.go`, inside the `r.Route("/api", ...)` block, add after the Config section:

```go
		// Deleted tracks management
		r.Get("/deleted", api.ListDeleted)
		r.Post("/deleted/restore", api.RestoreDeleted)
		r.Post("/deleted/purge", api.PurgeDeleted)
```

- [ ] **Step 4: Verify build**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/deleted.go internal/api/router.go
git commit -m "add deleted tracks API: list, restore, purge"
```

---

### Task 5: Add frontend API client methods

**Files:**
- Modify: `web/src/lib/api/client.ts`

- [ ] **Step 1: Add deleted track API methods**

Add to the `api` object in `client.ts`, after the config section:

```typescript
  // Deleted tracks
  listDeleted: () => request<Track[]>('GET', '/api/deleted'),
  restoreDeleted: (ids: number[]) =>
    request<{ status: string; restored: number }>('POST', '/api/deleted/restore', { ids }),
  purgeDeleted: (ids: number[], all = false) =>
    request<{ status: string; purged: number }>('POST', '/api/deleted/purge', all ? { all: true } : { ids }),
```

- [ ] **Step 2: Verify with svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/api/client.ts
git commit -m "add deleted tracks API client methods"
```

---

### Task 6: Create DeletedManager.svelte component

**Files:**
- Create: `web/src/lib/components/DeletedManager.svelte`

- [ ] **Step 1: Create the component**

```svelte
<script lang="ts">
  import { api, type Track } from '../api/client';
  import { ArrowLeft, RotateCcw, Trash2, CheckSquare, Square, CheckSquare2 } from 'lucide-svelte';

  let { onBack }: { onBack: () => void } = $props();

  let tracks = $state<Track[]>([]);
  let selected = $state<Set<number>>(new Set());
  let loading = $state(true);
  let error = $state('');
  let showPurgeConfirm = $state(false);

  let allSelected = $derived(tracks.length > 0 && selected.size === tracks.length);

  async function loadDeleted() {
    loading = true;
    error = '';
    try {
      tracks = (await api.listDeleted()) ?? [];
      selected = new Set();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  function toggleSelect(id: number) {
    const next = new Set(selected);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selected = next;
  }

  function toggleAll() {
    if (allSelected) {
      selected = new Set();
    } else {
      selected = new Set(tracks.map(t => t.id));
    }
  }

  async function restoreSelected() {
    if (selected.size === 0) return;
    error = '';
    try {
      await api.restoreDeleted([...selected]);
      await loadDeleted();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Restore failed';
    }
  }

  async function purgeSelected() {
    if (selected.size === 0) return;
    error = '';
    try {
      await api.purgeDeleted([...selected]);
      showPurgeConfirm = false;
      await loadDeleted();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Delete failed';
    }
  }

  loadDeleted();
</script>

<div class="deleted-manager">
  <header class="dm-header">
    <button class="back-btn" onclick={onBack} aria-label="Back">
      <ArrowLeft size={24} />
    </button>
    <h2>Marked for Deletion</h2>
    <span class="count">{tracks.length} files</span>
  </header>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  {#if loading}
    <div class="loading">Loading...</div>
  {:else if tracks.length === 0}
    <div class="empty">No files marked for deletion</div>
  {:else}
    <div class="actions-bar">
      <button class="select-all-btn" onclick={toggleAll}>
        {#if allSelected}
          <CheckSquare2 size={18} />
        {:else}
          <Square size={18} />
        {/if}
        <span>{allSelected ? 'Deselect all' : 'Select all'}</span>
      </button>

      {#if selected.size > 0}
        <button class="restore-btn" onclick={restoreSelected}>
          <RotateCcw size={16} />
          <span>Restore ({selected.size})</span>
        </button>
        <button class="purge-btn" onclick={() => showPurgeConfirm = true}>
          <Trash2 size={16} />
          <span>Delete ({selected.size})</span>
        </button>
      {/if}
    </div>

    <div class="track-list">
      {#each tracks as track (track.id)}
        <button class="track-item" class:selected={selected.has(track.id)} onclick={() => toggleSelect(track.id)}>
          <div class="checkbox">
            {#if selected.has(track.id)}
              <CheckSquare size={20} />
            {:else}
              <Square size={20} />
            {/if}
          </div>
          <div class="track-details">
            <span class="track-title">{track.title}</span>
            <span class="track-meta">
              {track.artist || 'Unknown'}
              {#if track.album} · {track.album}{/if}
              {#if track.play_count > 0} · {track.play_count}×{/if}
            </span>
          </div>
        </button>
      {/each}
    </div>
  {/if}

  {#if showPurgeConfirm}
    <div class="confirm-overlay" onclick={() => showPurgeConfirm = false}>
      <div class="confirm-dialog" onclick={(e) => e.stopPropagation()}>
        <p>Permanently delete {selected.size} file{selected.size !== 1 ? 's' : ''}?</p>
        <p class="confirm-warning">This cannot be undone.</p>
        <div class="confirm-actions">
          <button class="confirm-cancel" onclick={() => showPurgeConfirm = false}>Cancel</button>
          <button class="confirm-delete" onclick={purgeSelected}>Delete Forever</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .deleted-manager {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 0.75rem;
  }

  .dm-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.25rem 0.5rem;
    margin-bottom: 0.5rem;
  }

  .dm-header h2 {
    font-size: 1.1rem;
    margin: 0;
    flex: 1;
  }

  .count {
    font-size: 0.8rem;
    color: #888;
  }

  .back-btn {
    background: none;
    border: none;
    color: #f0f0f0;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 50%;
  }

  .actions-bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    flex-wrap: wrap;
  }

  .select-all-btn {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
  }

  .select-all-btn:hover {
    color: #f0f0f0;
    background: rgba(255, 255, 255, 0.1);
  }

  .restore-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: #1db954;
    border: none;
    color: white;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.8rem;
    border-radius: 16px;
    font-weight: 600;
    margin-left: auto;
  }

  .purge-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: #ff4444;
    border: none;
    color: white;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.8rem;
    border-radius: 16px;
    font-weight: 600;
  }

  .track-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .track-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a1a1a;
    border: none;
    border-radius: 8px;
    padding: 0.7rem 0.75rem;
    color: #f0f0f0;
    cursor: pointer;
    text-align: left;
  }

  .track-item:hover {
    background: #222;
  }

  .track-item.selected {
    background: #1a2a1a;
  }

  .checkbox {
    color: #555;
    flex-shrink: 0;
    display: flex;
  }

  .track-item.selected .checkbox {
    color: #1db954;
  }

  .track-details {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .track-title {
    font-size: 0.9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .track-meta {
    font-size: 0.75rem;
    color: #666;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .confirm-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .confirm-dialog {
    background: #1e1e1e;
    border-radius: 16px;
    padding: 1.5rem 2rem;
    text-align: center;
    min-width: 280px;
  }

  .confirm-dialog p {
    font-size: 1.05rem;
    margin: 0 0 0.5rem;
  }

  .confirm-warning {
    color: #ff6b6b;
    font-size: 0.85rem !important;
    margin-bottom: 1.25rem !important;
  }

  .confirm-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: center;
  }

  .confirm-cancel {
    background: #333;
    border: none;
    color: #f0f0f0;
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
  }

  .confirm-delete {
    background: #ff4444;
    border: none;
    color: white;
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
    font-weight: 600;
  }

  .loading, .empty, .error {
    text-align: center;
    padding: 2rem;
    color: #888;
  }

  .error {
    color: #ff6b6b;
    padding: 0.5rem;
  }
</style>
```

- [ ] **Step 2: Verify with svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/DeletedManager.svelte
git commit -m "add DeletedManager component for restore/purge UI"
```

---

### Task 7: Wire DeletedManager into Settings tab

**Files:**
- Modify: `web/src/lib/components/Settings.svelte`
- Modify: `web/src/App.svelte`

- [ ] **Step 1: Add deleted count and navigation to Settings.svelte**

Settings.svelte needs a new prop to open the deleted manager, and a button showing the deleted count.

Add to the script section:

```typescript
import { Trash2 } from 'lucide-svelte';

let { onDone, onOpenDeleted }: { onDone: () => void; onOpenDeleted?: () => void } = $props();

let deletedCount = $state(0);

async function loadDeletedCount() {
  try {
    const tracks = await api.listDeleted();
    deletedCount = tracks?.length ?? 0;
  } catch {
    // ignore
  }
}

loadDeletedCount();
```

Add a "Marked for Deletion" button in the template, after the `<div class="actions">` section (before the closing `</div>` of `.settings`):

```svelte
  {#if onOpenDeleted}
    <div class="section-divider"></div>
    <button class="deleted-btn" onclick={onOpenDeleted}>
      <Trash2 size={20} />
      <span>Marked for Deletion</span>
      {#if deletedCount > 0}
        <span class="deleted-count">{deletedCount}</span>
      {/if}
    </button>
  {/if}
```

Add CSS:

```css
  .section-divider {
    height: 1px;
    background: #222;
    margin: 1rem 0;
  }

  .deleted-btn {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a1a1a;
    border: 1px solid #333;
    border-radius: 12px;
    padding: 1rem;
    color: #f0f0f0;
    font-size: 1rem;
    cursor: pointer;
    width: 100%;
    text-align: left;
  }

  .deleted-btn:hover {
    border-color: #ff4444;
    background: #1e1e1e;
  }

  .deleted-count {
    margin-left: auto;
    background: #ff4444;
    color: white;
    font-size: 0.75rem;
    font-weight: 700;
    padding: 0.15rem 0.5rem;
    border-radius: 10px;
    min-width: 1.5rem;
    text-align: center;
  }
```

- [ ] **Step 2: Wire DeletedManager into App.svelte settings tab**

In `App.svelte`, import DeletedManager:

```typescript
import DeletedManager from './lib/components/DeletedManager.svelte';
```

Add a `showDeletedManager` state:

```typescript
let showDeletedManager = $state(false);
```

Replace the settings tab panel content:

```svelte
      <div class="tab-panel" class:hidden={activeTab !== 'settings'}>
        {#if showDeletedManager}
          <DeletedManager onBack={() => showDeletedManager = false} />
        {:else}
          <Settings onDone={() => activeTab = 'folders'} onOpenDeleted={() => showDeletedManager = true} />
        {/if}
      </div>
```

Also reset `showDeletedManager` when switching tabs — update the onTabChange handler:

```svelte
    <BottomNav {activeTab} onTabChange={(tab) => { activeTab = tab; showQueue = false; showDeletedManager = false; }} />
```

- [ ] **Step 3: Verify with svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/components/Settings.svelte web/src/App.svelte
git commit -m "wire DeletedManager into Settings tab"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 2: Run Go build**

```bash
go build ./...
```

- [ ] **Step 3: Verify manually**

Start backend and frontend. Test:
- Swipe left to reject a track
- Go to Settings > Marked for Deletion
- See the rejected track in the list
- Select it and click Restore — file moves back, track reappears in library
- Reject another track, select it, click Delete Forever with confirmation — file permanently deleted, art cache cleaned, empty folders cleaned up
