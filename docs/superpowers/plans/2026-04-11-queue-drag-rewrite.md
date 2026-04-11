# Queue Drag-to-Reorder Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken long-press drag system with a dedicated drag handle that works reliably on both mobile (touch) and desktop (mouse).

**Architecture:** A `GripVertical` drag handle gets `touch-action: none` so the browser never claims the gesture for scrolling. Touch and mouse events on the handle initiate drag immediately — no timers, no thresholds. Document-level move/end listeners track the gesture. Existing translateY visual shifting and auto-scroll logic are preserved.

**Tech Stack:** Svelte 5, TypeScript, lucide-svelte (GripVertical icon), Playwright (tests)

**Spec:** `docs/superpowers/specs/2026-04-11-queue-drag-rewrite-design.md`

---

### Task 1: Remove old long-press drag system and add drag handle

**Files:**
- Modify: `web/src/lib/components/QueueView.svelte`

- [ ] **Step 1: Replace imports and constants**

In the `<script>` section, change the imports and constants. Remove the old timing constants and add the mouse activation threshold.

Replace lines 1-11:
```svelte
<script lang="ts">
  import { tick, onDestroy } from 'svelte';
  import { api, type Track } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';
  import { ArrowLeft, ChevronUp, ChevronDown, Play, GripVertical } from 'lucide-svelte';

  const HAPTIC_DURATION_MS = 30;
  const MOUSE_DRAG_THRESHOLD_PX = 3;
```

- [ ] **Step 2: Replace drag state variables**

Remove the old drag state block (lines ~32-41) and replace with:

```typescript
  // Drag state
  let dragOriginalIndex = $state<number | null>(null);
  let targetIndex = $state<number | null>(null);
  let isDragging = $state(false);
  let dragDeltaY = $state(0);
  let dragOriginY = $state(0);
  let dragScrollStart = 0;
  let itemHeight = 56;

  // Mouse drag: track pending state before threshold is met
  let mouseDownY = 0;
  let mouseDragPending = false;
  let pendingMouseIndex: number | null = null;
```

- [ ] **Step 3: Remove old touch handlers and holdTimer**

Delete the entire `handleTouchStart`, `handleTouchMove`, `handleTouchEnd`, `cancelHold` functions, the `holdTimer` variable, the `userScrolledAt` state, and the `SCROLL_SUPPRESS_MS`-related scroll suppression in `scrollToCurrent`.

Remove `userScrolledAt` from the state declarations and simplify `scrollToCurrent`:

```typescript
  async function scrollToCurrent(force = false) {
    await tick();
    if (!listEl) return;
    const currentEl = listEl.querySelector('.queue-item.current') as HTMLElement;
    if (currentEl) {
      currentEl.scrollIntoView({ block: 'center', behavior: 'instant' });
    }
  }
```

- [ ] **Step 4: Add shared drag helpers**

These are used by both touch and mouse code paths. Add after `measureItemHeight`:

```typescript
  function startDrag(idx: number, clientY: number) {
    isDragging = true;
    dragOriginalIndex = idx;
    targetIndex = idx;
    dragOriginY = clientY;
    dragScrollStart = listEl?.scrollTop ?? 0;
    dragDeltaY = 0;
    measureItemHeight();
    if (navigator.vibrate) navigator.vibrate(HAPTIC_DURATION_MS);
  }

  function updateDrag(clientY: number) {
    dragDeltaY = clientY - dragOriginY;
    handleEdgeScroll(clientY);
    const scrollDelta = (listEl?.scrollTop ?? 0) - dragScrollStart;
    const delta = dragDeltaY + scrollDelta;
    const indexOffset = Math.round(delta / itemHeight);
    targetIndex = Math.max(0, Math.min(tracks.length - 1, (dragOriginalIndex ?? 0) + indexOffset));
  }

  function finishDrag() {
    stopAutoScroll();
    if (isDragging && dragOriginalIndex !== null && targetIndex !== null) {
      if (dragOriginalIndex !== targetIndex) {
        moveTrack(dragOriginalIndex, targetIndex);
        saveOrder();
      }
    }
    isDragging = false;
    dragOriginalIndex = null;
    targetIndex = null;
    dragDeltaY = 0;
    dragOriginY = 0;
  }
```

- [ ] **Step 5: Add touch drag handlers (for the grip handle)**

These register document-level listeners with `{ passive: false }`:

```typescript
  // --- Touch drag (mobile) ---
  function handleGripTouchStart(e: TouchEvent, idx: number) {
    const touch = e.touches[0];
    startDrag(idx, touch.clientY);
    document.addEventListener('touchmove', handleDocTouchMove, { passive: false });
    document.addEventListener('touchend', handleDocTouchEnd);
    document.addEventListener('touchcancel', handleDocTouchEnd);
  }

  function handleDocTouchMove(e: TouchEvent) {
    if (!isDragging) return;
    e.preventDefault();
    updateDrag(e.touches[0].clientY);
  }

  function handleDocTouchEnd() {
    document.removeEventListener('touchmove', handleDocTouchMove);
    document.removeEventListener('touchend', handleDocTouchEnd);
    document.removeEventListener('touchcancel', handleDocTouchEnd);
    finishDrag();
  }
```

- [ ] **Step 6: Add mouse drag handlers (for desktop testing)**

Mouse drag uses a 3px threshold to distinguish click from drag:

```typescript
  // --- Mouse drag (desktop) ---
  function handleGripMouseDown(e: MouseEvent, idx: number) {
    e.preventDefault();
    mouseDownY = e.clientY;
    mouseDragPending = true;
    pendingMouseIndex = idx;
    document.addEventListener('mousemove', handleDocMouseMove);
    document.addEventListener('mouseup', handleDocMouseUp);
  }

  function handleDocMouseMove(e: MouseEvent) {
    if (mouseDragPending && !isDragging) {
      if (Math.abs(e.clientY - mouseDownY) >= MOUSE_DRAG_THRESHOLD_PX) {
        mouseDragPending = false;
        startDrag(pendingMouseIndex!, mouseDownY);
      } else {
        return;
      }
    }
    if (!isDragging) return;
    e.preventDefault();
    updateDrag(e.clientY);
  }

  function handleDocMouseUp() {
    document.removeEventListener('mousemove', handleDocMouseMove);
    document.removeEventListener('mouseup', handleDocMouseUp);
    mouseDragPending = false;
    pendingMouseIndex = null;
    finishDrag();
  }
```

- [ ] **Step 7: Add cleanup on destroy**

```typescript
  onDestroy(() => {
    document.removeEventListener('touchmove', handleDocTouchMove);
    document.removeEventListener('touchend', handleDocTouchEnd);
    document.removeEventListener('touchcancel', handleDocTouchEnd);
    document.removeEventListener('mousemove', handleDocMouseMove);
    document.removeEventListener('mouseup', handleDocMouseUp);
    stopAutoScroll();
  });
```

- [ ] **Step 8: Update the template**

Replace the `{#each}` block (lines 271-327). The key changes:
- Remove `ontouchstart`/`ontouchmove`/`ontouchend`/`ontouchcancel` from `.queue-item`
- Add a `.drag-handle` div with `GripVertical` icon before the chevron buttons
- Attach touch/mouse handlers to the `.drag-handle`

```svelte
      {#each tracks as track, idx (track.id)}
        <div
          class="queue-item"
          class:current={track.id === currentTrackId}
          class:dragging={isDragging && dragOriginalIndex === idx}
          data-testid="queue-item"
          data-track-id={track.id}
          role="button"
          tabindex="0"
          style={getItemStyle(idx)}
          onclick={() => skipTo(track.id)}
          onkeydown={(e) => { if (e.key === 'Enter') skipTo(track.id); }}
        >
          <div class="track-indicator">
            {#if track.id === currentTrackId}
              <Play size={14} fill="#4ec484" color="#4ec484" />
            {:else}
              <span class="track-num">{idx + 1}</span>
            {/if}
          </div>

          <div class="track-details">
            <span class="track-title">{track.title}</span>
            <span class="track-meta">
              {track.artist || 'Unknown'}
              {#if track.play_count > 0}
                · {track.play_count}×
              {/if}
            </span>
          </div>

          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="drag-handle"
            data-testid="drag-handle"
            ontouchstart={(e) => handleGripTouchStart(e, idx)}
            onmousedown={(e) => handleGripMouseDown(e, idx)}
          >
            <GripVertical size={20} />
          </div>

          <div class="move-buttons">
            <button
              class="move-btn"
              onclick={(e) => moveUp(idx, e)}
              disabled={idx === 0}
              aria-label="Move up"
              data-testid="move-up"
            >
              <ChevronUp size={18} />
            </button>
            <button
              class="move-btn"
              onclick={(e) => moveDown(idx, e)}
              disabled={idx === tracks.length - 1}
              aria-label="Move down"
              data-testid="move-down"
            >
              <ChevronDown size={18} />
            </button>
          </div>
        </div>
      {/each}
```

- [ ] **Step 9: Update the drag hint text**

Replace the drag hint block (lines 253-257):

```svelte
  {#if isDragging}
    <div class="drag-hint">Release to drop</div>
  {:else}
    <div class="drag-hint subtle">Drag ≡ to reorder · Tap to play</div>
  {/if}
```

- [ ] **Step 10: Update the scroll handler on queue-list**

Remove the `userScrolledAt` tracking from the scroll handler since we removed that system:

```svelte
    <div
      class="queue-list"
      class:dragging-active={isDragging}
      data-testid="queue-list"
      bind:this={listEl}
    >
```

- [ ] **Step 11: Commit script changes**

```bash
# Don't commit yet — CSS changes come next
```

---

### Task 2: Update CSS for drag handle

**Files:**
- Modify: `web/src/lib/components/QueueView.svelte` (style section)

- [ ] **Step 1: Remove `touch-action: pan-y` from `.queue-item` and add drag handle styles**

In the `<style>` section, make these changes:

Remove `touch-action: pan-y;` from `.queue-item` (line 403).

Add these new rules after the `.move-btn:disabled` rule (around line 508):

```css
  .drag-handle {
    touch-action: none;
    cursor: grab;
    color: #555;
    padding: 0.5rem;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    border-radius: 4px;
  }

  .drag-handle:active {
    cursor: grabbing;
    color: var(--color-text);
  }

  .queue-item.dragging .drag-handle {
    cursor: grabbing;
  }
```

- [ ] **Step 2: Verify the existing `.queue-item.dragging` rule**

The existing rule at lines 429-435 sets `touch-action: none` on the dragged item. This should stay — it prevents any residual browser interference during an active drag. Verify it still has:

```css
  .queue-item.dragging {
    background: #2a2a2a;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
    z-index: 10;
    border-radius: 12px;
    touch-action: none;
  }
```

- [ ] **Step 3: Run svelte-check**

Run: `cd web && npx svelte-check`
Expected: 0 errors, 0 warnings

- [ ] **Step 4: Run frontend build**

Run: `cd web && npm run build`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/QueueView.svelte
git commit -m "Rewrite queue drag-to-reorder with dedicated grip handle"
```

---

### Task 3: Manual mouse testing with dev server

**Files:** None (testing only)

- [ ] **Step 1: Start the dev server**

Run: `cd web && npm run dev`

Open the URL shown (typically `http://localhost:5173`) in a browser.

- [ ] **Step 2: Navigate to queue**

Navigate to Now Playing tab → tap Queue button. Verify the queue loads with tracks visible.

- [ ] **Step 3: Test mouse drag**

Grab the `≡` grip icon with the mouse on any track. Drag up/down. Verify:
- The list does NOT scroll while dragging
- Other items shift with smooth transitions to show the drop gap
- The dragged item follows the cursor
- Releasing drops the item at the correct position

- [ ] **Step 4: Test click-vs-drag distinction**

Click the grip icon without moving the mouse. Verify:
- No drag is triggered (3px threshold)
- The track does NOT skip (click on grip should not trigger skipTo)

Click the track body (title/artist area). Verify:
- It triggers skip-to-track as expected

- [ ] **Step 5: Test chevron fallback**

Click the up/down chevron buttons. Verify they still reorder tracks correctly.

- [ ] **Step 6: Test edge auto-scroll**

If the queue has many tracks, drag an item to the top or bottom edge of the visible area. Verify auto-scroll engages and items scroll while dragging.

---

### Task 4: Update Playwright tests

**Files:**
- Modify: `web/tests/queue.spec.ts`

- [ ] **Step 1: Add a drag handle reorder test**

Add after the existing `move-down` test (after line 143):

```typescript
  test('drag handle reorders track via mouse', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    const items = page.locator('[data-testid="queue-item"]');
    const count = await items.count();
    if (count < 3) {
      test.skip();
      return;
    }

    // Get the ID of the third track
    const thirdId = await items.nth(2).getAttribute('data-track-id');

    // Get the bounding boxes of the third and first items
    const thirdHandle = items.nth(2).locator('[data-testid="drag-handle"]');
    const firstItem = items.nth(0);
    const handleBox = await thirdHandle.boundingBox();
    const firstBox = await firstItem.boundingBox();

    if (!handleBox || !firstBox) {
      test.skip();
      return;
    }

    // Drag from third item's handle to first item's position
    const startX = handleBox.x + handleBox.width / 2;
    const startY = handleBox.y + handleBox.height / 2;
    const endY = firstBox.y + firstBox.height / 2;

    await page.mouse.move(startX, startY);
    await page.mouse.down();
    // Move in steps to exceed threshold and trigger reorder
    await page.mouse.move(startX, startY - 10, { steps: 2 });
    await page.mouse.move(startX, endY, { steps: 10 });
    await page.mouse.up();

    await page.waitForTimeout(500);

    // The third track should now be at position 1 (index 0)
    const firstId = await items.nth(0).getAttribute('data-track-id');
    expect(firstId).toBe(thirdId);
  });
```

- [ ] **Step 2: Add a test verifying drag handle exists on all items**

Add after the previous test:

```typescript
  test('all queue items have a drag handle', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    const items = page.locator('[data-testid="queue-item"]');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      await expect(items.nth(i).locator('[data-testid="drag-handle"]')).toBeVisible();
    }
  });
```

- [ ] **Step 3: Commit test changes**

```bash
git add web/tests/queue.spec.ts
git commit -m "Add Playwright test for drag handle reorder"
```

---

### Task 5: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update queue section in README if it mentions drag behavior**

Check if README mentions queue reordering. If it does, update the description to mention the grip handle. If not, no change needed.

- [ ] **Step 2: Commit if changed**

```bash
git add README.md
git commit -m "Update README for queue drag handle"
```
