import { test, expect, type Page } from '@playwright/test';

/**
 * Stubs every API endpoint the app hits at startup and during
 * the settings dir-selection flow, without a live backend.
 */
async function stubAllApis(page: Page) {
  // Config — music dir is /music, device connected
  await page.route('**/api/config', async route => {
    if (route.request().method() === 'GET') {
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
    } else {
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{"status":"ok"}' });
    }
  });

  // Player state — idle, connected
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
  await page.route('**/api/devices/scan', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify([{ name: 'Fake Renderer', udn: 'fake-udn', location: 'http://localhost' }]),
    });
  });
  await page.route('**/api/devices', async route => {
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

  // Deleted count
  await page.route('**/api/deleted', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  // Scan status — idle
  await page.route('**/api/library/scan/status', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        scanning: false,
        scanned: 0,
        total: 0,
        phase: '',
        analyzing: false,
        analyzed: 0,
        analysis_total: 0,
        analysis_error: '',
      }),
    });
  });

  // Folders — empty
  await page.route('**/api/folders**', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  // Tracks — empty
  await page.route('**/api/tracks**', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  // Browse — returns a directory different from musicDir so the select button is enabled
  await page.route('**/api/browse**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        current: '/music/new',
        parent: '/music',
        entries: [],
      }),
    });
  });

  // Browse shortcuts
  await page.route('**/api/browse/shortcuts', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
  });

  // Set music dir — POST
  await page.route('**/api/config/music-dir', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok', music_dir: '/music/new', delete_dir: '/music/new/.deleted' }),
    });
  });

  // Device info for settings panel
  await page.route('**/api/devices/current', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ name: 'Fake Renderer', udn: 'fake-udn' }),
    });
  });
}

test('setting a new music dir does not auto-navigate away from settings', async ({ page }) => {
  await stubAllApis(page);

  await page.goto('/');

  // Wait for main phase
  const bottomNav = page.locator('.bottom-nav');
  await expect(bottomNav).toBeVisible({ timeout: 15_000 });

  // Go to Settings tab (3rd tab)
  await page.locator('.nav-tab').nth(2).click();
  await expect(page.locator('.settings')).toBeVisible();

  // Open Music Directory browser (use the specific settings-item button)
  await page.locator('.settings .settings-item').filter({ hasText: 'Music Directory' }).click();
  // Wait for the browser panel to appear
  await expect(page.locator('.browser')).toBeVisible({ timeout: 5_000 });

  // The select button should show and be enabled (currentPath=/music/new != musicDir=/music)
  const selectBtn = page.locator('.select-btn');
  await expect(selectBtn).toBeVisible();
  await expect(selectBtn).toBeEnabled();

  // Click the select button to choose the new directory
  await selectBtn.click();

  // Give time for any navigation to happen
  await page.waitForTimeout(500);

  // The Settings panel should still be visible — we should NOT have navigated to folders
  const settingsPanel = page.locator('.tab-panel').filter({ has: page.locator('.settings') });
  await expect(settingsPanel).not.toHaveClass(/hidden/);
});
