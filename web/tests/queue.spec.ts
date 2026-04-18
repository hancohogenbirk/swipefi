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

  test('drag handle is the first child of each queue item', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    const items = page.locator('[data-testid="queue-item"]');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      // The drag handle should be the first child element of the queue item
      const firstChild = items.nth(i).locator('> :first-child');
      await expect(firstChild).toHaveAttribute('data-testid', 'drag-handle');
    }
  });

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

  test('back button returns to now playing from queue', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    await page.locator('.queue-view .back-btn').click();
    await expect(page.locator('.now-playing')).toBeVisible();
  });

  test('queue updates current highlight after skip-to', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    // Get the current track ID
    const currentBefore = await page.locator('.queue-item.current').getAttribute('data-track-id');

    // Click a non-current track
    const nonCurrent = page.locator('[data-testid="queue-item"]:not(.current)').first();
    const targetId = await nonCurrent.getAttribute('data-track-id');
    await nonCurrent.click();

    // Wait for the current highlight to move to the clicked track
    await expect(async () => {
      const currentAfter = await page.locator('.queue-item.current').getAttribute('data-track-id');
      expect(currentAfter).toBe(targetId);
    }).toPass({ timeout: 5_000 });
  });

  test('scroll position preserved after swipe-to-remove', async ({ page }) => {
    await ensurePlaying(page);
    await page.locator('.queue-btn').click();
    await expect(page.locator('.queue-view')).toBeVisible();

    const items = page.locator('[data-testid="queue-item"]');
    const count = await items.count();
    if (count < 5) {
      test.skip();
      return;
    }

    // Scroll down so the current track is NOT visible
    const queueList = page.locator('.queue-list');
    await queueList.evaluate(el => el.scrollTop = el.scrollHeight);
    await page.waitForTimeout(300);

    // Record scroll position before swipe
    const scrollBefore = await queueList.evaluate(el => el.scrollTop);

    // Find a non-current item near the bottom and swipe it left (reject)
    const lastNonCurrent = page.locator('[data-testid="queue-item"]:not(.current)').last();
    const box = await lastNonCurrent.boundingBox();
    if (!box) { test.skip(); return; }

    // Perform swipe left gesture
    await page.mouse.move(box.x + box.width * 0.8, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x - 100, box.y + box.height / 2, { steps: 5 });
    await page.mouse.up();
    await page.waitForTimeout(800);

    // Scroll position should be approximately the same (within 200px tolerance for collapse)
    const scrollAfter = await queueList.evaluate(el => el.scrollTop);
    const scrollDelta = Math.abs(scrollAfter - scrollBefore);
    expect(scrollDelta).toBeLessThan(200);
  });
});
