# Group 3: Deletion & Undelete

## Summary

Add permanent deletion of rejected files, restore (unmark) functionality, centralize the `to_delete` folder, clean up empty folders and cached art on delete.

## Centralized `to_delete` Folder

- Always at `<music_dir>/to_delete` — derived from the music_dir setting, not a separate config.
- Created automatically when music_dir is set or changed.
- Old `to_delete` folders in previous music_dir locations are left alone.
- Remove the `SWIPEFI_DELETE_DIR` env var override and `delete_dir` DB config key. The delete dir is always `filepath.Join(musicDir, "to_delete")`.
- Update `Player`, `main.go`, and `API` config endpoints to derive delete_dir instead of storing it separately.

## Permanent Delete

**Endpoint:** `POST /api/deleted/purge`  
**Body:** `{ "ids": [1, 2, 3] }` or `{ "all": true }`  
**Response:** `{ "status": "ok", "purged": 3 }`

For each track:
1. Delete the file from `<music_dir>/to_delete/<track.Path>`.
2. Delete cached cover art: `data/art/<id>.jpg`, `data/art/<id>.png`, `data/art/<id>.noart`.
3. Hard-delete the track row from the database (not soft-delete — `DELETE FROM tracks WHERE id = ?`).
4. Clean up the original source folder: if `<music_dir>/<dir_of_track.Path>` is now empty, delete it. Walk up the directory tree deleting empty parents until reaching a non-empty folder or the music_dir root.
5. Clean up inside `to_delete`: if the containing folder inside `to_delete` is now empty, delete it and walk up similarly until reaching the `to_delete` root.

## Unmark / Restore

**Endpoint:** `POST /api/deleted/restore`  
**Body:** `{ "ids": [1, 2, 3] }`  
**Response:** `{ "status": "ok", "restored": 3 }`

For each track:
1. The original path is implicit: `<music_dir>/<track.Path>`. The file currently lives at `<music_dir>/to_delete/<track.Path>`.
2. Create parent directories at the original location if needed (`os.MkdirAll`).
3. Move the file back: `os.Rename(to_delete_path, original_path)`.
4. Set `deleted = 0` in the database.
5. Clean up empty folders inside `to_delete` after the move.

## List Deleted Tracks

**Endpoint:** `GET /api/deleted`  
**Response:** Array of Track objects where `deleted = 1`, including id, path, title, artist, album, play_count.

## Settings UI — "Marked for Deletion" Section

In the Settings tab, add a "Marked for Deletion (N)" button. Tapping opens a `DeletedManager.svelte` sub-view:

- List of deleted tracks showing title, artist, album, play count.
- Checkboxes for multi-select.
- "Select All" toggle at the top.
- "Restore Selected" button (green) — calls restore API, refreshes list.
- "Delete Forever" button (red) — shows confirmation dialog: "This will permanently delete N files. This cannot be undone." On confirm, calls purge API.
- Back button returns to settings.

## Delete Cached Art Helper

Add a `DeleteCachedArt(cacheDir string, trackID int64)` function in `internal/api/art.go` that removes `<id>.jpg`, `<id>.png`, and `<id>.noart` from the cache dir. Called during purge.

## Store Changes

- `ListDeleted(ctx) ([]Track, error)` — `SELECT ... FROM tracks WHERE deleted = 1`
- `PurgeTrack(ctx, id) error` — `DELETE FROM tracks WHERE id = ?`
- `UnmarkDeleted(ctx, id) error` — `UPDATE tracks SET deleted = 0 WHERE id = ?`

## Folder Cleanup Helper

A shared utility function `CleanupEmptyDirs(dir string, stopAt string)` that:
1. If `dir` is empty, removes it.
2. Walks up to parent, repeats.
3. Stops when reaching `stopAt` (never deletes `stopAt` itself).

Used by both purge (clean original folder + to_delete folder) and restore (clean to_delete folder).

## Config Endpoint Change

`GET /api/config` currently returns `{ music_dir, delete_dir }`. Change to derive `delete_dir` from `music_dir`:
- Response still includes `delete_dir` for frontend reference, but it's always `<music_dir>/to_delete`.
- `POST /api/config/music-dir` no longer needs to set `delete_dir` in the DB.

## Non-Goals

- Browsing `to_delete` as a folder in the Folders tab (it stays hidden from the scanner).
- Undo after permanent deletion.
- Batch restore by folder.
