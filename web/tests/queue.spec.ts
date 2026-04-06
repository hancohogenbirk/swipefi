import { test, expect, type Page } from '@playwright/test';

// Requires Go backend running with music dir on port 8080

test.describe.serial('Queue management', () => {
  const API = 'http://localhost:8080';

  async function ensurePlaying(page: Page) {
    // Set up playback via API directly — more reliable than navigating UI
    const devices = await (await page.request.get(`${API}/api/devices`)).json();
    if (devices.length > 0) {
      await page.request.post(`${API}/api/devices/select`, {
        data: { udn: devices[0].udn },
      });
    }

    const playerState = await (await page.request.get(`${API}/api/player/state`)).json();
    if (playerState.state === 'idle') {
      // Get first folder dynamically
      const folders = await (await page.request.get(`${API}/api/folders`)).json();
      expect(folders.length).toBeGreaterThan(0);
      await page.request.post(`${API}/api/player/play`, {
        data: { folder: folders[0].path, sort: 'added_at', order: 'asc' },
      });
    }

    await page.goto('/');

    // Wait for the app to finish loading — either bottom nav or setup screen
    const bottomNav = page.locator('.bottom-nav');
    const setup = page.locator('.center-screen');
    await expect(bottomNav.or(setup)).toBeVisible({ timeout: 15_000 });

    // If on setup, select device
    if (await setup.isVisible()) {
      const deviceBtn = page.locator('.device-btn').first();
      if (await deviceBtn.isVisible()) {
        await deviceBtn.click();
      }
      await expect(bottomNav).toBeVisible({ timeout: 10_000 });
    }

    // Switch to Now Playing tab
    await page.locator('.nav-tab').nth(1).click();
    await expect(page.locator('.now-playing')).toBeVisible({ timeout: 5_000 });
  }

  test('queue button opens queue view', async ({ page }) => {
    await ensurePlaying(page);

    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();
    await expect(page.locator('.queue-header h2')).toHaveText('Queue');
  });

  test('queue shows tracks with current highlighted', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    // Should have multiple tracks
    const items = page.locator('[data-testid="queue-item"]');
    await expect(items.first()).toBeVisible();
    const count = await items.count();
    expect(count).toBeGreaterThan(1);

    // One track should be marked as current
    await expect(page.locator('.queue-item.current')).toBeVisible();
  });

  test('skip-to plays a different track', async ({ page }) => {
    await ensurePlaying(page);

    // Get current track via API
    const before = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );
    const beforeId = before.track?.id;

    // Open queue
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    // Click a non-current track (find one that isn't highlighted)
    const nonCurrentItem = page.locator('[data-testid="queue-item"]:not(.current)').first();
    await expect(nonCurrentItem).toBeVisible();
    await nonCurrentItem.click();
    await page.waitForTimeout(1000);

    // Should now be playing a different track
    const after = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );
    expect(after.track?.id).not.toBe(beforeId);
  });

  test('move-up button reorders track', async ({ page }) => {
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

    // Click move-up on the third track
    await items.nth(2).locator('[data-testid="move-up"]').click();
    await page.waitForTimeout(500);

    // The third track should now be at position 2 (index 1)
    const secondId = await items.nth(1).getAttribute('data-track-id');
    expect(secondId).toBe(thirdId);
  });

  test('move-down button reorders track', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    const items = page.locator('[data-testid="queue-item"]');
    const count = await items.count();
    if (count < 3) {
      test.skip();
      return;
    }

    // Get the ID of the first track
    const firstId = await items.nth(0).getAttribute('data-track-id');

    // Click move-down on the first track
    await items.nth(0).locator('[data-testid="move-down"]').click();
    await page.waitForTimeout(500);

    // The first track should now be at position 2 (index 1)
    const secondId = await items.nth(1).getAttribute('data-track-id');
    expect(secondId).toBe(firstId);
  });

  test('back button returns to now playing from queue', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    await page.locator('.queue-view .back-btn').click();
    await expect(page.locator('.now-playing')).toBeVisible();
  });

  test('queue updates after skip-to', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    const itemsBefore = await page.locator('[data-testid="queue-item"]').count();

    // Skip to the second track (removes the first)
    const items = page.locator('[data-testid="queue-item"]');
    const secondItem = items.nth(1);
    await secondItem.click();

    // Wait for queue to refresh reactively (queue reloads when track changes)
    await expect(async () => {
      const itemsAfter = await page.locator('[data-testid="queue-item"]').count();
      expect(itemsAfter).toBeLessThan(itemsBefore);
    }).toPass({ timeout: 5_000 });
  });
});
