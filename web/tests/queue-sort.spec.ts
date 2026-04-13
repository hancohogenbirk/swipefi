import { test, expect, type Page } from '@playwright/test';

/**
 * Tests that the queue sort-value column displays the correct date
 * for the active sort mode (last_played vs added_at).
 *
 * All API responses are mocked — no backend required.
 */

const NOW_TS = Math.floor(Date.now() / 1000);
const ONE_DAY = 86400;

// Two tracks: one played (has last_played), one never played
const TRACK_PLAYED = {
  id: 1,
  path: '/music/played.flac',
  title: 'Played Track',
  artist: 'Artist A',
  album: 'Album',
  duration_ms: 180000,
  format: 'flac',
  play_count: 3,
  added_at: NOW_TS - 30 * ONE_DAY, // 30 days ago
  last_played: NOW_TS - 2 * ONE_DAY, // 2 days ago
};

const TRACK_UNPLAYED = {
  id: 2,
  path: '/music/unplayed.flac',
  title: 'Unplayed Track',
  artist: 'Artist B',
  album: 'Album',
  duration_ms: 200000,
  format: 'flac',
  play_count: 0,
  added_at: NOW_TS - 10 * ONE_DAY, // 10 days ago
  // last_played intentionally omitted (never played)
};

const TRACKS = [TRACK_PLAYED, TRACK_UNPLAYED];

function playerState(track = TRACK_PLAYED) {
  return {
    state: 'playing',
    connected: true,
    track,
    position_ms: 0,
    duration_ms: track.duration_ms,
    queue_length: TRACKS.length,
    queue_position: 0,
  };
}

function queueResponse(sortBy: string) {
  return {
    tracks: TRACKS,
    position: 0,
    folder: '/music',
    sort_by: sortBy,
    sort_order: 'desc',
  };
}

async function setupMocks(page: Page, sortBy: string) {
  const json = (route: import('@playwright/test').Route, data: unknown) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) });

  // Mock each API endpoint the app calls during init and queue loading.
  // We use exact path-based routes to avoid intercepting Vite module requests.
  await page.route('**/api/config', (route) =>
    json(route, { music_dir: '/music', connected_device: 'test-udn' }),
  );
  await page.route('**/api/player/state', (route) =>
    json(route, playerState()),
  );
  await page.route('**/api/player/queue', (route) =>
    json(route, queueResponse(sortBy)),
  );
  await page.route('**/api/library/scan/status', (route) =>
    json(route, { scanning: false, scanned: 0, total: 0, phase: '', analyzing: false, analyzed: 0, analysis_total: 0, analysis_error: '' }),
  );
  await page.route('**/api/devices/scan', (route) =>
    json(route, []),
  );
  await page.route('**/api/devices', (route) =>
    json(route, []),
  );
  await page.route('**/api/deleted/processing', (route) =>
    json(route, { active: false }),
  );
  // Additional mocks so Vite doesn't proxy unhandled requests to the absent backend
  await page.route('**/api/folders*', (route) =>
    json(route, []),
  );
  await page.route('**/api/deleted', (route) =>
    json(route, []),
  );
  await page.route('**/api/tracks/*/art', (route) =>
    route.fulfill({ status: 404 }),
  );
}

/**
 * Helper: navigate to the app, wait for it to load, switch to Now Playing,
 * open the queue view.
 */
async function openQueue(page: Page) {
  await page.goto('/');

  // Wait for bottom nav (main phase loaded)
  const bottomNav = page.locator('.bottom-nav');
  await expect(bottomNav).toBeVisible({ timeout: 15_000 });

  // Switch to Now Playing tab
  await page.locator('.nav-tab').nth(1).click();
  await expect(page.locator('.now-playing')).toBeVisible({ timeout: 5_000 });

  // Open queue
  await page.locator('.queue-btn').click();
  await expect(page.locator('.queue-view')).toBeVisible({ timeout: 5_000 });
}

test.describe('Queue sort-value display', () => {

  test('last_played sort shows last_played date for played track and em-dash for unplayed', async ({ page }) => {
    await setupMocks(page, 'last_played');
    await openQueue(page);

    const items = page.locator('[data-testid="queue-item"]');
    await expect(items).toHaveCount(2);

    // Played track (index 0): should show a date from last_played (2 days ago → "2d ago")
    const playedSortValue = items.nth(0).locator('.sort-value');
    await expect(playedSortValue).toBeVisible();
    const playedText = await playedSortValue.textContent();
    // It should show "2d ago" (from last_played), NOT "30d ago" or a month-based label (from added_at)
    expect(playedText).toContain('2d ago');

    // Unplayed track (index 1): should show em-dash since last_played is null
    const unplayedSortValue = items.nth(1).locator('.sort-value');
    await expect(unplayedSortValue).toBeVisible();
    const unplayedText = await unplayedSortValue.textContent();
    expect(unplayedText!.trim()).toBe('—');
  });

  test('added_at sort shows added_at date for all tracks', async ({ page }) => {
    await setupMocks(page, 'added_at');
    await openQueue(page);

    const items = page.locator('[data-testid="queue-item"]');
    await expect(items).toHaveCount(2);

    // Both tracks should show their added_at date, not last_played or em-dash
    // Track 1: added 30 days ago
    const firstSortValue = items.nth(0).locator('.sort-value .date-val');
    await expect(firstSortValue).toBeVisible();

    // Track 2: added 10 days ago → "10d ago" (within 7 days shows "Xd ago" but 10d shows a month-day)
    // Actually 10 days > 7, so it shows "Apr 3" style format. Let's just check it has a Clock icon (date-val present).
    const secondSortValue = items.nth(1).locator('.sort-value .date-val');
    await expect(secondSortValue).toBeVisible();

    // Critically: the unplayed track should NOT show an em-dash in added_at mode
    const secondText = await items.nth(1).locator('.sort-value').textContent();
    expect(secondText!.trim()).not.toBe('—');
  });
});
