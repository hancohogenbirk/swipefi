import { test, expect } from '@playwright/test';

test('settings renders all sections even when /api/config is slow/fails', async ({ page }) => {
  let configCallCount = 0;

  // First call to /api/config succeeds (App.svelte onMount needs it to reach 'main' phase).
  // Subsequent calls (Settings.svelte loadConfig + loadDeviceInfo) abort to simulate hang/failure.
  await page.route('**/api/config', async route => {
    configCallCount++;
    if (configCallCount === 1) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          music_dir: '/music',
          delete_dir: '/music/.deleted',
          connected_device: 'Test Speaker',
          flacalyzer_available: false,
          flacalyzer_enabled: false,
        }),
      });
    } else {
      // Simulate hang/failure for Settings internal calls
      await new Promise(r => setTimeout(r, 100));
      await route.abort();
    }
  });

  await page.route('**/api/library/scan/status', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ scanning: false, analyzing: false, analyzed: 0, analysis_total: 0, analysis_error: '' }),
    });
  });

  await page.route('**/api/player/state', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ state: 'idle', connected: true, position_ms: 0, duration_ms: 0, queue_length: 0, queue_position: 0 }),
    });
  });

  await page.route('**/api/devices', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  await page.route('**/api/devices/scan', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  await page.route('**/api/deleted', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  await page.route('**/api/deleted/processing', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ active: false }),
    });
  });

  await page.route('**/api/folders**', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  await page.goto('/');
  const bottomNav = page.locator('.bottom-nav');
  await expect(bottomNav).toBeVisible({ timeout: 15_000 });

  // Settings tab (3rd)
  await page.locator('.nav-tab').nth(2).click();
  await expect(page.locator('.settings')).toBeVisible();

  // All five sections visible (use .item-label spans to avoid ambiguity)
  await expect(page.locator('.settings .item-label', { hasText: 'Music Directory' })).toBeVisible();
  await expect(page.locator('.settings .item-label', { hasText: /Rescan Library|Rescanning/ })).toBeVisible();
  await expect(page.locator('.settings .item-label', { hasText: 'Transcode Detection' })).toBeVisible();
  await expect(page.locator('.settings .item-label', { hasText: 'Marked for Deletion' })).toBeVisible();
  await expect(page.locator('.settings .item-label', { hasText: 'Audio Device' })).toBeVisible();
});
