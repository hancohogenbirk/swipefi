# Navigation & Playback Accuracy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ad-hoc view routing with Spotify-style bottom tab navigation, fix play count bugs, detect external device takeover, and migrate all icons to Lucide.

**Architecture:** App.svelte becomes a shell with a persistent BottomNav and MiniPlayer. Tab content renders conditionally with state preserved. History API manages in-tab back navigation. Backend Player gains stream URL tracking and updates play count in the queue's in-memory track objects.

**Tech Stack:** Svelte 5, TypeScript, lucide-svelte, Go (player/queue changes)

---

### Task 1: Install lucide-svelte

**Files:**
- Modify: `web/package.json`

- [ ] **Step 1: Install the package**

```bash
cd web && npm install lucide-svelte
```

- [ ] **Step 2: Verify installation**

```bash
cd web && node -e "require('lucide-svelte'); console.log('OK')"
```

Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "add lucide-svelte icon library"
```

---

### Task 2: Migrate all inline SVGs to Lucide icons

**Files:**
- Modify: `web/src/lib/components/TransportControls.svelte`
- Modify: `web/src/lib/components/NowPlaying.svelte`
- Modify: `web/src/lib/components/FolderNav.svelte`
- Modify: `web/src/lib/components/QueueView.svelte`
- Modify: `web/src/lib/components/SwipeCard.svelte`
- Modify: `web/src/lib/components/Settings.svelte`

Do NOT change the SwipeFi gradient text logo in App.svelte.

- [ ] **Step 1: Migrate TransportControls.svelte**

Replace all inline SVGs with Lucide components. Add import at top of script:

```svelte
<script lang="ts">
  import { SkipBack, RotateCcw, Play, Pause, RotateCw, SkipForward } from 'lucide-svelte';
```

Replace the five button contents:
- Previous button SVG → `<SkipBack size={28} />`
- Back 15s button SVG → `<RotateCcw size={32} />` (keep the `<span class="skip-label">15</span>`)
- Play/Pause button SVGs → `<Play size={36} fill="currentColor" />` / `<Pause size={36} fill="currentColor" />`
- Forward 15s button SVG → `<RotateCw size={32} />` (keep the skip-label)
- Next button SVG → `<SkipForward size={28} />`

- [ ] **Step 2: Migrate NowPlaying.svelte**

```svelte
<script lang="ts">
  import { ArrowLeft, ListMusic } from 'lucide-svelte';
```

- Back button SVG → `<ArrowLeft size={24} />`
- Queue button SVG → `<ListMusic size={22} />`

- [ ] **Step 3: Migrate FolderNav.svelte**

```svelte
<script lang="ts">
  import { Home, Settings, Folder, ArrowUp, Play } from 'lucide-svelte';
```

- Home button SVG → `<Home size={18} />`
- Settings button SVG → `<Settings size={20} />`
- Folder emoji `📁` → `<Folder size={20} />`
- Up arrow emoji `⬆` → `<ArrowUp size={20} />`
- Play button text `▶` → `<Play size={16} fill="currentColor" />`
- Play all icon `▶` → `<Play size={18} fill="currentColor" />`

- [ ] **Step 4: Migrate QueueView.svelte**

```svelte
<script lang="ts">
  import { ArrowLeft, ChevronUp, ChevronDown, Play } from 'lucide-svelte';
```

- Back button SVG → `<ArrowLeft size={24} />`
- Now-playing icon `▶` → `<Play size={14} fill="#1db954" />`
- Move up button SVG → `<ChevronUp size={18} />`
- Move down button SVG → `<ChevronDown size={18} />`

- [ ] **Step 5: Migrate SwipeCard.svelte**

```svelte
<script lang="ts">
  import { Music } from 'lucide-svelte';
```

- Music note placeholder SVG → `<Music size={64} />`

- [ ] **Step 6: Migrate Settings.svelte**

```svelte
<script lang="ts">
  import { X, Folder, ArrowUp, Zap } from 'lucide-svelte';
```

- Close button `✕` → `<X size={20} />`
- Folder emoji `📁` → `<Folder size={20} />`
- Up arrow emoji `⬆` → `<ArrowUp size={20} />`
- Shortcut emoji `⚡` → `<Zap size={20} />`

- [ ] **Step 7: Verify the dev server compiles**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

Expected: No errors.

- [ ] **Step 8: Commit**

```bash
git add web/src/
git commit -m "migrate all icons to lucide-svelte"
```

---

### Task 3: Create BottomNav component

**Files:**
- Create: `web/src/lib/components/BottomNav.svelte`

- [ ] **Step 1: Create the component**

```svelte
<script lang="ts">
  import { FolderOpen, Disc3, Settings } from 'lucide-svelte';

  type Tab = 'folders' | 'player' | 'settings';

  let { activeTab, onTabChange }: {
    activeTab: Tab;
    onTabChange: (tab: Tab) => void;
  } = $props();
</script>

<nav class="bottom-nav">
  <button
    class="nav-tab"
    class:active={activeTab === 'folders'}
    onclick={() => onTabChange('folders')}
    aria-label="Folders"
  >
    <FolderOpen size={22} />
    <span class="nav-label">Folders</span>
  </button>

  <button
    class="nav-tab"
    class:active={activeTab === 'player'}
    onclick={() => onTabChange('player')}
    aria-label="Now Playing"
  >
    <Disc3 size={22} />
    <span class="nav-label">Now Playing</span>
  </button>

  <button
    class="nav-tab"
    class:active={activeTab === 'settings'}
    onclick={() => onTabChange('settings')}
    aria-label="Settings"
  >
    <Settings size={22} />
    <span class="nav-label">Settings</span>
  </button>
</nav>

<style>
  .bottom-nav {
    display: flex;
    justify-content: space-around;
    align-items: center;
    padding: 0.4rem 0 0.5rem;
    background: #0d0d0d;
    border-top: 1px solid #222;
    flex-shrink: 0;
  }

  .nav-tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    padding: 0.25rem 1rem;
    border-radius: 8px;
    font-size: 0.65rem;
    transition: color 0.15s;
  }

  .nav-tab.active {
    color: #1db954;
  }

  .nav-tab:hover:not(.active) {
    color: #aaa;
  }

  .nav-label {
    font-weight: 500;
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/components/BottomNav.svelte
git commit -m "add BottomNav component"
```

---

### Task 4: Create MiniPlayer component (extract from App.svelte)

**Files:**
- Create: `web/src/lib/components/MiniPlayer.svelte`

- [ ] **Step 1: Create the component**

Extract the existing mini-player markup from App.svelte into its own component:

```svelte
<script lang="ts">
  import { getPlayerState } from '../stores/player.svelte';

  let { onClick }: { onClick: () => void } = $props();

  let ps = $derived(getPlayerState());
  let visible = $derived(ps.state !== 'idle' && ps.track);
</script>

{#if visible && ps.track}
  <button class="mini-player" onclick={onClick}>
    <img
      src="/api/tracks/{ps.track.id}/art"
      alt=""
      class="mini-art"
      onerror={(e) => (e.currentTarget as HTMLImageElement).style.display = 'none'}
    />
    <div class="mini-info">
      <span class="mini-title">{ps.track.title}</span>
      <span class="mini-artist">{ps.track.artist || 'Unknown'}</span>
    </div>
    <span class="mini-state">{ps.state === 'playing' ? '▶' : '⏸'}</span>
    <div class="mini-progress" style="width: {ps.duration_ms ? (ps.position_ms / ps.duration_ms * 100) : 0}%"></div>
  </button>
{/if}

<style>
  .mini-player {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a1a1a;
    border: none;
    border-top: 1px solid #333;
    color: #f0f0f0;
    padding: 0.6rem 1rem;
    cursor: pointer;
    width: 100%;
    text-align: left;
    position: relative;
    overflow: hidden;
    flex-shrink: 0;
  }

  .mini-art {
    width: 42px;
    height: 42px;
    border-radius: 6px;
    object-fit: cover;
    flex-shrink: 0;
  }

  .mini-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }

  .mini-title {
    font-size: 0.9rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mini-artist {
    font-size: 0.75rem;
    color: #888;
  }

  .mini-state {
    font-size: 1.2rem;
    flex-shrink: 0;
  }

  .mini-progress {
    position: absolute;
    bottom: 0;
    left: 0;
    height: 2px;
    background: #1db954;
    transition: width 1s linear;
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/components/MiniPlayer.svelte
git commit -m "extract MiniPlayer component from App.svelte"
```

---

### Task 5: Rewrite App.svelte with tab navigation

**Files:**
- Modify: `web/src/App.svelte`

This is the big task. App.svelte becomes a shell: loading/setup flows remain as-is, but once setup is done the app shows tab content + mini-player + bottom nav.

- [ ] **Step 1: Rewrite App.svelte**

Key changes:
- Replace the `View` type. Split into `appPhase: 'loading' | 'choose-dir' | 'setup' | 'main'` and `activeTab: 'folders' | 'player' | 'settings'`.
- In the `'main'` phase, render all three tab contents but only show the active one (use `display: none` to preserve state when switching tabs, particularly the FolderNav folder path).
- Add MiniPlayer above BottomNav (visible when a track is playing, tapping switches to player tab).
- Queue is a sub-view within the player tab: add `showQueue` boolean state.
- Remove the `onNavigateToPlayer` / `onBack` / `onOpenSettings` / `navigateToHome` callback spaghetti — tabs handle that now.
- Remove mini-player inline markup (now in MiniPlayer component).
- Remove settings gear from FolderNav props (it's a tab now).
- Remove home button from FolderNav props (device selection moves to settings).

New structure for the main phase:

```svelte
<div class="app">
  <!-- Pre-navigation phases (unchanged) -->
  {#if appPhase === 'loading'}
    <!-- existing loading screen -->
  {:else if appPhase === 'choose-dir'}
    <Settings onDone={onMusicDirChosen} />
  {:else if appPhase === 'setup'}
    <!-- existing device selection screen -->
  {:else}
    <!-- Main app with tabs -->
    <div class="tab-content">
      <div class="tab-panel" class:hidden={activeTab !== 'folders'}>
        {#if scanProgress.scanning}
          <!-- scan banner (existing) -->
        {/if}
        <FolderNav onNavigateToPlayer={() => activeTab = 'player'} />
      </div>

      <div class="tab-panel" class:hidden={activeTab !== 'player'}>
        {#if showQueue}
          <QueueView onBack={() => showQueue = false} />
        {:else}
          <NowPlaying onBack={() => activeTab = 'folders'} onOpenQueue={() => showQueue = true} />
        {/if}
      </div>

      <div class="tab-panel" class:hidden={activeTab !== 'settings'}>
        <Settings onDone={() => activeTab = 'folders'} />
      </div>
    </div>

    <MiniPlayer onClick={() => activeTab = 'player'} />
    <BottomNav {activeTab} onTabChange={(tab) => { activeTab = tab; if (tab !== 'player') showQueue = false; }} />
  {/if}
</div>
```

Add CSS:
```css
.tab-content {
  flex: 1;
  min-height: 0;
  position: relative;
}

.tab-panel {
  height: 100%;
  overflow: hidden;
}

.tab-panel.hidden {
  display: none;
}
```

- [ ] **Step 2: Update FolderNav.svelte — remove settings/home props**

Remove `onOpenSettings` and `onNavigateHome` from the props interface. Remove the settings gear icon button and the home button from the header. Keep `onNavigateToPlayer` (used when pressing play on a folder).

Update props:
```typescript
let { onNavigateToPlayer }: {
  onNavigateToPlayer: () => void;
} = $props();
```

Remove from the header template: the `home-btn` button with its SVG, and the `icon-btn` settings button. Keep breadcrumbs and sort select.

- [ ] **Step 3: Hide MiniPlayer when on the player tab**

In App.svelte, only render MiniPlayer when `activeTab !== 'player'`:

```svelte
{#if activeTab !== 'player'}
  <MiniPlayer onClick={() => activeTab = 'player'} />
{/if}
```

- [ ] **Step 4: Verify with svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/
git commit -m "add bottom tab navigation, restructure App.svelte"
```

---

### Task 6: Add History API back button support

**Files:**
- Modify: `web/src/App.svelte`

- [ ] **Step 1: Add history management to App.svelte**

Add the following logic to the `<script>` section after the existing state declarations:

```typescript
// --- History API for back button ---
let folderHistory = $state<string[]>([]);  // stack of folder paths navigated into

function pushFolderHistory(path: string) {
  folderHistory = [...folderHistory, path];
  history.pushState({ type: 'folder', path }, '');
}

function pushQueueHistory() {
  history.pushState({ type: 'queue' }, '');
}

let showExitConfirm = $state(false);

function handlePopState(e: PopStateEvent) {
  // Queue sub-view: go back to now playing
  if (showQueue) {
    showQueue = false;
    return;
  }

  // Folder navigation: go back to parent folder
  if (activeTab === 'folders' && folderHistory.length > 0) {
    folderHistory = folderHistory.slice(0, -1);
    // FolderNav will react to this via a callback
    return;
  }

  // At tab root — show exit confirmation
  // Push a state back so we don't actually leave
  history.pushState(null, '');
  showExitConfirm = true;
}
```

In `onMount`, add:
```typescript
window.addEventListener('popstate', handlePopState);
// Push initial state so first back press doesn't leave
history.pushState(null, '');
```

In `onDestroy`, add:
```typescript
window.removeEventListener('popstate', handlePopState);
```

- [ ] **Step 2: Add exit confirmation dialog**

After the BottomNav in the template, add:

```svelte
{#if showExitConfirm}
  <div class="exit-overlay" onclick={() => showExitConfirm = false}>
    <div class="exit-dialog" onclick={(e) => e.stopPropagation()}>
      <p>Leave SwipeFi?</p>
      <div class="exit-actions">
        <button class="exit-cancel" onclick={() => showExitConfirm = false}>Cancel</button>
        <button class="exit-leave" onclick={() => history.back()}>Leave</button>
      </div>
    </div>
  </div>
{/if}
```

CSS:
```css
.exit-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.exit-dialog {
  background: #1e1e1e;
  border-radius: 16px;
  padding: 1.5rem 2rem;
  text-align: center;
  min-width: 250px;
}

.exit-dialog p {
  font-size: 1.1rem;
  margin: 0 0 1.25rem;
}

.exit-actions {
  display: flex;
  gap: 0.75rem;
  justify-content: center;
}

.exit-cancel {
  background: #333;
  border: none;
  color: #f0f0f0;
  padding: 0.6rem 1.5rem;
  border-radius: 24px;
  font-size: 0.95rem;
  cursor: pointer;
}

.exit-leave {
  background: #ff4444;
  border: none;
  color: white;
  padding: 0.6rem 1.5rem;
  border-radius: 24px;
  font-size: 0.95rem;
  cursor: pointer;
}
```

- [ ] **Step 3: Wire folder navigation to history**

Update FolderNav.svelte to accept two new optional props for history integration:

Add to FolderNav props:
```typescript
let { onNavigateToPlayer, onFolderNavigate }: {
  onNavigateToPlayer: () => void;
  onFolderNavigate?: (path: string) => void;
} = $props();
```

In the `navigateTo` and `navigateToBreadcrumb` functions, call `onFolderNavigate?.(path)` after `loadFolders(path)`. Do NOT call it for `navigateUp` (back is handled by popstate).

In App.svelte, pass the callback:
```svelte
<FolderNav
  onNavigateToPlayer={() => activeTab = 'player'}
  onFolderNavigate={pushFolderHistory}
/>
```

- [ ] **Step 4: Wire queue to history**

When opening the queue, push history:
```svelte
onOpenQueue={() => { showQueue = true; pushQueueHistory(); }}
```

- [ ] **Step 5: Verify with svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 6: Commit**

```bash
git add web/src/
git commit -m "add History API back button with exit confirmation"
```

---

### Task 7: Backend — update play count in queue's in-memory track

**Files:**
- Modify: `internal/player/player.go`
- Modify: `internal/player/queue.go`

- [ ] **Step 1: Add UpdatePlayCount to Queue**

In `internal/player/queue.go`, add:

```go
// UpdateCurrentPlayCount updates the play count of the current track in-memory.
func (q *Queue) UpdateCurrentPlayCount(count int) {
	if q.pos >= 0 && q.pos < len(q.tracks) {
		q.tracks[q.pos].PlayCount = count
	}
}
```

- [ ] **Step 2: Update checkPlayCountLocked to refresh in-memory track**

In `internal/player/player.go`, in `checkPlayCountLocked`, after `p.store.IncrementPlayCount(ctx, track.ID)`, add:

```go
// Update in-memory track so WebSocket broadcast shows new count
track.PlayCount++
q := p.queue
if q != nil {
    q.UpdateCurrentPlayCount(track.PlayCount)
}
```

Replace the existing block:
```go
	if shouldCount {
		track := p.queue.Current()
		if track != nil {
			p.store.IncrementPlayCount(ctx, track.ID)
			slog.Info("play count incremented", "track_id", track.ID, "title", track.Title)
		}
		p.playCounted = true
	}
```

With:
```go
	if shouldCount {
		track := p.queue.Current()
		if track != nil {
			p.store.IncrementPlayCount(ctx, track.ID)
			track.PlayCount++
			if p.queue != nil {
				p.queue.UpdateCurrentPlayCount(track.PlayCount)
			}
			slog.Info("play count incremented", "track_id", track.ID, "title", track.Title, "new_count", track.PlayCount)
		}
		p.playCounted = true
	}
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/player/
git commit -m "update in-memory track play count for live WebSocket updates"
```

---

### Task 8: Backend — detect external device takeover

**Files:**
- Modify: `internal/player/player.go`

- [ ] **Step 1: Add currentStreamURL field**

Add a new field to the Player struct:

```go
// Expected stream URL for the current track
currentStreamURL string
```

- [ ] **Step 2: Set it in playCurrentLocked**

In `playCurrentLocked`, after building the `streamURL`, store it:

```go
p.currentStreamURL = streamURL
```

Add this right after the `streamURL := fmt.Sprintf(...)` line.

- [ ] **Step 3: Check in pollOnce**

In `pollOnce`, after acquiring the lock and updating `p.positionMs` / `p.durationMs`, add a check before the existing "track ended naturally" check:

```go
// Check if another source took over the device
if pos.TrackURI != "" && p.currentStreamURL != "" && pos.TrackURI != p.currentStreamURL {
    slog.Info("external source took over device", "expected", p.currentStreamURL, "actual", pos.TrackURI)
    p.state = StateIdle
    p.currentStreamURL = ""
    p.stopPollingLocked()
    p.notify()
    return
}
```

This goes right after `p.durationMs = pos.TrackDuration.Milliseconds()` and before the `if tState == dlna.StateStopped` check.

- [ ] **Step 4: Clear on stop**

In the places where state transitions to idle (end of queue in `Next`, `Reject`, and `pollOnce` natural end), also clear `currentStreamURL`:

```go
p.currentStreamURL = ""
```

- [ ] **Step 5: Verify build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/player/player.go
git commit -m "detect external device takeover, transition to idle"
```

---

### Task 9: Frontend — reactive queue refresh after swipe

**Files:**
- Modify: `web/src/lib/components/QueueView.svelte`

- [ ] **Step 1: Add reactive effect to reload queue**

In QueueView.svelte, add an `$effect` that watches for track changes and reloads:

```typescript
let lastTrackId = $state<number | undefined>(undefined);

$effect(() => {
  const currentId = ps.track?.id;
  if (currentId !== undefined && currentId !== lastTrackId) {
    lastTrackId = currentId;
    loadQueue();
  }
});
```

This ensures the queue reloads whenever the playing track changes (from swipe, skip, or natural advancement).

- [ ] **Step 2: Verify with svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/components/QueueView.svelte
git commit -m "reactively refresh queue when current track changes"
```

---

### Task 10: Investigate and fix play count on swipe right + restart

**Files:**
- Modify: `internal/player/player.go`

- [ ] **Step 1: Add debug logging to trace play count state**

Add slog.Debug calls at key points to trace the `playCounted` flag:

In `playCurrentLocked`, after `p.playCounted = false`:
```go
slog.Debug("playCurrentLocked: reset playCounted", "track_id", track.ID, "title", track.Title)
```

In `checkPlayCountLocked`, at entry:
```go
slog.Debug("checkPlayCountLocked", "playCounted", p.playCounted, "force", force)
```

In `Next`, before `checkPlayCountLocked`:
```go
slog.Debug("Next called", "current_track", p.queue.Current().Title, "playCounted", p.playCounted)
```

In `SkipToTrack`, before `checkPlayCountLocked`:
```go
slog.Debug("SkipToTrack called", "target_id", trackID, "playCounted", p.playCounted)
```

- [ ] **Step 2: Analyze the flow and fix**

The flow for "swipe right on track A, then tap track A in queue":

1. `Next()` → `checkPlayCountLocked(true)` counts track A, sets `playCounted=true` → `queue.Next()` advances to B → `playCurrentLocked` plays B, resets `playCounted=false`
2. `SkipToTrack(A)` → `checkPlayCountLocked(true)` counts track B (playCounted was false), sets `playCounted=true` → `queue.SkipTo(A)` → `playCurrentLocked` plays A, resets `playCounted=false`

This flow should work correctly — track A gets counted in step 1, track B gets counted in step 2, and track A starts fresh with `playCounted=false`. If the user swipes right again on track A, `Next()` will call `checkPlayCountLocked(true)` which sees `playCounted=false` and increments.

However, there's a subtle issue: `SkipTo` removes all tracks before the target. If the user taps the currently playing track (not a different one), `SkipTo` still calls `checkPlayCountLocked(true)` and then `playCurrentLocked` — which restarts the track and resets counting. This is correct behavior.

The actual bug may be that `SkipTo` in queue.go removes tracks before the target, including ones already played. If track A is at position 3 and user skips to it, tracks 0-2 are removed. Verify this doesn't cause issues with the `Next()` call finding track A already gone.

Keep the debug logging, remove it in a future cleanup if the bug is confirmed fixed.

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/player/player.go
git commit -m "add play count debug logging, verify swipe-right-restart flow"
```

---

### Task 11: Final verification

- [ ] **Step 1: Run svelte-check**

```bash
cd web && npx svelte-check --tsconfig ./tsconfig.json
```

- [ ] **Step 2: Run Go build**

```bash
go build ./...
```

- [ ] **Step 3: Run frontend dev server and check manually**

```bash
cd web && npm run dev
```

Verify in browser:
- Bottom nav shows 3 tabs with Lucide icons
- Switching tabs preserves folder state
- Back button navigates within tabs
- Exit confirmation shows at tab root
- Mini-player visible above bottom nav when track is playing
- Queue refreshes when track changes

- [ ] **Step 4: Run Go tests if any exist**

```bash
go test ./...
```
