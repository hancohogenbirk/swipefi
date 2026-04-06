# Groups 1 & 2: Navigation Rework + Playback State Accuracy

## Summary

Replace the current ad-hoc view state with a Spotify-style bottom tab navigation bar and fix four playback state accuracy bugs. Add Lucide Icons as the icon library, replacing all inline SVGs.

## Icon Library

Install `lucide-svelte` (ISC license, free/open source). Replace all existing hand-crafted inline SVGs across the app with Lucide components. This gives consistent sizing, stroke width, and visual style.

**Exception:** The SwipeFi gradient text logo (`.logo` class) stays unchanged.

**Icon mapping (current inline SVGs → Lucide):**
- Back arrow → `ArrowLeft`
- Queue/playlist icon → `ListMusic`
- Home icon → `Home`
- Settings gear → `Settings`
- Play → `Play`, Pause → `Pause`
- Skip forward/back → `SkipForward` / `SkipBack`
- Rewind/fast-forward 15s → `RotateCcw` / `RotateCw` (or custom)
- Folder → `Folder`
- Up arrow (navigate up) → `ChevronUp` or `ArrowUp`
- Music note placeholder (no art) → `Music`
- Move up/down in queue → `ChevronUp` / `ChevronDown`
- Bottom nav tabs: `FolderOpen`, `Disc3` (now playing), `Settings`

## Group 1: Navigation

### Bottom Navigation Bar

A persistent bottom nav bar with three tabs: **Folders**, **Now Playing**, **Settings**. Visible on all screens except the initial loading/setup flow (before a device is selected).

**Structure (bottom to top):**
1. **Bottom nav** — 3 tab buttons, active tab highlighted in green (#1db954), inactive in #888. Fixed at bottom.
2. **Mini-player** — sits directly above the bottom nav (like Spotify). Visible when a track is or was playing (state !== idle OR track is loaded). Tapping it switches to the Now Playing tab.
3. **Page content** — fills remaining space above.

**Tab contents:**
- **Folders tab**: Current FolderNav component (breadcrumbs, folder list, play buttons, sort selector). Settings gear icon moves from the FolderNav header to the Settings tab.
- **Now Playing tab**: Current NowPlaying component. When no track has been played yet, shows an empty state ("No track playing — browse your folders to start"). The queue view is a sub-screen within this tab.
- **Settings tab**: Current Settings component (music dir picker). The "Home" button (device selection) also moves here as a "Change device" option.

**Tab switching:**
- Tapping a tab switches instantly (no animation needed).
- Tab state is preserved: navigating away from Folders and back should keep the current folder path.
- The setup/loading/choose-dir views remain full-screen flows without the bottom nav (they're pre-navigation).

### Tab-Based Back Button (History API)

Use `history.pushState` / `popstate` to make the browser back button work within the app.

**Rules:**
- Switching between tabs does NOT push to the history stack.
- Navigation within a tab pushes to the stack:
  - Folders: navigating into a subfolder pushes state.
  - Now Playing: opening the queue pushes state.
  - Settings: no sub-views currently.
- Pressing back pops the stack and navigates within the current tab.
- At a tab's root (no more history within that tab), pressing back shows a confirmation dialog: "Leave SwipeFi?" with Cancel/Leave buttons. This prevents accidentally closing the app.

**Implementation approach:**
- Maintain a per-tab history stack in the app state.
- On `popstate`, check if the current tab has history to go back to. If yes, pop it. If no, show the exit confirmation.
- On navigation within a tab, `pushState` with the tab and sub-state info.

## Group 2: Playback State Accuracy

### Bug 1: Play count not incrementing on swipe right + queue restart

**Scenario:** User swipes right (Next) on track A → play count increments, queue advances to track B. User opens queue, taps track A again → track A plays but play count does NOT increment on the next swipe.

**Root cause:** The flow is: `Next()` calls `checkPlayCountLocked(true)` → increments count, sets `playCounted=true` → advances queue → plays track B. Then `SkipTo(trackA)` calls `checkPlayCountLocked(true)` for track B (counts it), then `playCurrentLocked` for track A which resets `playCounted=false`. This sequence actually works correctly for the *next* play count. The real issue to investigate: does the swipe-right path (which calls `Next()` directly, not `SkipTo`) have a case where `playCounted` is already true from a previous play of the same track? Verify during implementation with debug logging on the exact `playCounted` state transitions.

### Bug 2: Queue doesn't reflect current track after swipe

**Root cause:** QueueView fetches the queue once on mount (`loadQueue()` at line 145) but never refreshes when the player state changes (e.g., after swipe left/right changes the current track).

**Fix:** Add a reactive effect in QueueView that watches `ps.track?.id`. When it changes, reload the queue from the API. Use `$effect` to trigger this.

### Feature: Live play count on Now Playing screen

**Current state:** The `PlayerState` struct includes a `Track` pointer, which has `PlayCount`. But the track object in the queue is a snapshot from when the queue was built — it doesn't update when `IncrementPlayCount` is called.

**Fix:** After `IncrementPlayCount` is called in `checkPlayCountLocked`, also update the track's `PlayCount` field in the queue's in-memory copy. This way the next `notify()` broadcast includes the updated count, and the frontend SwipeCard shows the new value in real time.

### Bug 3: Track keeps playing in UI while device plays another source

**Root cause:** The poll loop checks `transport.GetState()` for STOPPED to detect natural track end, but doesn't verify that the renderer is still playing *our* track. If another app (e.g., Spotify) takes over the device, the renderer reports PLAYING with a different URI, and SwipeFi doesn't notice.

**Fix:** In `pollOnce`, after getting the position info, compare `pos.TrackURI` with the expected stream URL for the current track. If they don't match and the state is PLAYING or PAUSED, another source took over. Transition to idle state: stop polling, set state to idle, notify. Don't increment play count in this case.

Store the current expected stream URL as a field on Player (set in `playCurrentLocked`). Compare in `pollOnce`.

## Non-Goals

- No animated tab transitions (keep it snappy).
- No per-tab scroll position restoration (nice-to-have, not in scope).
- Queue drag/scroll fix (Group 7, separate spec).
- Now Playing UI polish (Group 5, separate spec).
