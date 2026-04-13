import { test, expect, type Page } from '@playwright/test';

/**
 * Stubs every API endpoint the app hits at startup so the test
 * runs without a live backend.
 */
async function stubAllApis(page: Page) {
  // Config — pretend music dir is set and a device is connected
  await page.route('**/api/config', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        music_dir: '/music',
        delete_dir: '/music/.deleted',
        connected_device: 'fake-udn',
        flacalyzer_available: false,
        flacalyzer_enabled: false,
      }),
    });
  });

  // Player state — idle
  await page.route('**/api/player/state', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        state: 'idle',
        connected: true,
        position_ms: 0,
        duration_ms: 0,
        queue_length: 0,
        queue_position: 0,
      }),
    });
  });

  // Devices
  await page.route('**/api/devices', async route => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([{ name: 'Fake Renderer', udn: 'fake-udn', location: 'http://localhost' }]),
      });
    } else {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
    }
  });
  await page.route('**/api/devices/scan', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([{ name: 'Fake Renderer', udn: 'fake-udn', location: 'http://localhost' }]),
    });
  });

  // Deleted processing — not active
  await page.route('**/api/deleted/processing', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ active: false }),
    });
  });

  // Scan status: scanning done, analyzing in progress
  await page.route('**/api/library/scan/status', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        scanning: false,
        scanned: 100,
        total: 100,
        phase: 'done',
        analyzing: true,
        analyzed: 10,
        analysis_total: 100,
        analysis_error: '',
      }),
    });
  });

  // Folders — return empty so FolderNav enters the "no content" path
  // that checks scan status
  await page.route('**/api/folders**', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  // Tracks — return empty
  await page.route('**/api/tracks**', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  // WebSocket — stub as a no-op (Playwright can't truly mock WS,
  // but we can prevent connection errors by intercepting the upgrade)
}

test('folder tab shows scan progress UI when only analyzing (not scanning)', async ({ page }) => {
  await stubAllApis(page);

  await page.goto('/');

  // Wait for main phase (bottom nav visible)
  const bottomNav = page.locator('.bottom-nav');
  await expect(bottomNav).toBeVisible({ timeout: 15_000 });

  // Navigate to Folders tab (first nav-tab)
  await page.locator('.nav-tab').first().click();

  // Should show scan/analysis progress UI, NOT plain "Loading..."
  // FolderNav uses .scan-progress and .scan-bar for progress display
  await expect(page.locator('.scan-progress')).toBeVisible({ timeout: 5_000 });
  await expect(page.locator('.scan-bar')).toBeVisible({ timeout: 5_000 });

  // The plain "Loading..." text should NOT be showing
  await expect(page.locator('.loading')).not.toBeVisible();
});
