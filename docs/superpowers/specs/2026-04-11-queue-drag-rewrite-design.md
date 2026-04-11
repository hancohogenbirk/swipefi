# Queue Drag-to-Reorder Rewrite

## Problem

Queue drag-and-drop reordering has been unreliable across 9+ fix attempts. Two persistent issues:
1. The list scrolls when you start dragging
2. Dragging triggers too easily when scrolling

Root cause: the current long-press approach on the entire queue item conflicts with `touch-action: pan-y` at the W3C spec level. The browser claims the gesture for scrolling at `touchstart` time, before the 250ms hold timer fires. No amount of timer/threshold tuning fixes this.

## Solution

Replace the long-press system with a **dedicated drag handle** (`GripVertical` icon). The handle has `touch-action: none` so the browser never claims scrolling on it. The rest of the item scrolls normally.

This is the standard pattern used by Spotify, Apple Music, YouTube Music, and recommended by dnd-kit, SortableJS.

## File

`web/src/lib/components/QueueView.svelte` — single file rewrite of drag logic.

## Layout (per queue item)

```
┌──────────────────────────────────────────────┐
│  [#]  Title                    [≡]  [▲] [▼] │
│       Artist · 3×                            │
└──────────────────────────────────────────────┘
  ↑     ↑                        ↑    ↑
  indicator  track details    grip   chevrons
  (tap = skip to track)     handle  (fallback)
```

- Grip handle: `GripVertical` from lucide-svelte, placed left of existing chevrons
- Chevrons kept as fallback
- Tap on item body = skip to track (unchanged)

## Drag Handle Behavior

### Touch (mobile)

1. `touchstart` on `.drag-handle`:
   - Set `isDragging = true`, record `dragOriginalIndex`, `touchStartY`, `dragScrollStart`
   - Measure `itemHeight` from DOM
   - Fire `navigator.vibrate(30)`
   - Register `touchmove` + `touchend` on `document` with `{ passive: false }`

2. `touchmove` on `document`:
   - `e.preventDefault()` — works because handle has `touch-action: none`
   - Update `dragDeltaY = touch.clientY - dragOriginY`
   - Compute `targetIndex` from delta + scroll offset
   - Auto-scroll if finger near top/bottom edge (existing logic)

3. `touchend` on `document`:
   - If `dragOriginalIndex !== targetIndex`: move track, save order to backend
   - Clean up all drag state
   - Remove document-level listeners

### Mouse (desktop / local testing)

Same flow using `mousedown` on `.drag-handle`, `mousemove`/`mouseup` on `document`. No long-press delay — drag starts on first `mousemove` after `mousedown`.

Small activation threshold (3px movement) to distinguish click from drag on mouse, preventing accidental drags on handle click.

## CSS

```css
.queue-item {
  /* Default touch-action — browser handles scroll on item body */
  user-select: none;
}

.drag-handle {
  touch-action: none;    /* Browser hands gesture control to JS */
  cursor: grab;
  padding: 0.5rem;       /* Generous touch target */
}

.drag-handle:active {
  cursor: grabbing;
}

.queue-item.dragging {
  touch-action: none;    /* Prevent any browser interference during drag */
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
  z-index: 10;
}
```

## What Gets Removed

- `HOLD_DELAY_MS`, `DRAG_JITTER_PX`, `SCROLL_SUPPRESS_MS` constants
- `holdTimer`, `cancelHold()` — entire long-press timer system
- `touchStartY` tracking for jitter detection
- `userScrolledAt` scroll suppression timestamp
- `touch-action: pan-y` on `.queue-item`
- Touch handlers (`ontouchstart`/`ontouchmove`/`ontouchend`) on `.queue-item`

## What Stays

- `dragOriginalIndex`, `targetIndex`, `isDragging`, `dragDeltaY`, `dragOriginY` state
- `getItemStyle()` — translateY shifting for visual gap
- `handleEdgeScroll()` / `stopAutoScroll()` — auto-scroll near edges
- `moveTrack()` / `saveOrder()` — reorder logic
- `moveUp()` / `moveDown()` — chevron button fallback
- Haptic feedback on drag start
- `scrollToCurrent()` auto-scroll to now-playing on load

## Visual Feedback

- Dragged item: elevated with box-shadow, follows finger/mouse directly (no transition)
- Other items: shift up/down with 150ms transition to show drop gap
- Drag hint text changes: "Drag ≡ to reorder · Tap to play"

## Testing

### Local (mouse)

Run `cd web && npm run dev` and open in browser. Grab the grip icon with mouse, drag up/down. Verify:
- List does NOT scroll while dragging
- Items shift to show drop gap
- Release drops item at correct position
- Clicking the grip without moving does not trigger drag (3px threshold)
- Clicking track body still triggers skip-to-track

### Mobile

Open on phone. Touch the grip icon and drag. Verify:
- List scrolls normally when touching item body
- Drag activates only from grip handle
- No accidental drags while scrolling
- Haptic feedback on drag start
- Auto-scroll works near edges
- Drop finalizes at correct position

### Automated

Existing Playwright tests in `web/tests/queue.spec.ts` should be updated to target the drag handle element instead of the queue item.
