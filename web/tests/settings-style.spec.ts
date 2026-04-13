import { test, expect } from '@playwright/test';

/** Helper: set up API routes so App.svelte reaches 'main' phase and Settings renders with flacalyzer data */
async function mockSettingsWithFlacalyzer(page: import('@playwright/test').Page, options: { analyzing: boolean; analyzed: number; analysisTotal: number; flacalyzerEnabled: boolean }) {
  await page.route('**/api/config', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        music_dir: '/music',
        delete_dir: '/music/.deleted',
        connected_device: 'Test Speaker',
        flacalyzer_available: true,
        flacalyzer_enabled: options.flacalyzerEnabled,
      }),
    });
  });

  await page.route('**/api/library/scan/status', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        scanning: false,
        analyzing: options.analyzing,
        analyzed: options.analyzed,
        analysis_total: options.analysisTotal,
        analysis_error: '',
      }),
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

  await page.route('**/api/deleted**', async route => {
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
}

test('flacalyzer progress bar uses universal green-to-blue gradient', async ({ page }) => {
  await mockSettingsWithFlacalyzer(page, { analyzing: true, analyzed: 50, analysisTotal: 100, flacalyzerEnabled: true });

  await page.goto('/');
  await expect(page.locator('.bottom-nav')).toBeVisible({ timeout: 15_000 });
  await page.locator('.nav-tab').nth(2).click();

  const fill = page.locator('.analysis-fill').first();
  await expect(fill).toBeVisible();
  // Universal gradient: linear-gradient(90deg, #1db954, #7cb3ff)
  // In computed styles this becomes linear-gradient(90deg, rgb(29, 185, 84), rgb(124, 179, 255))
  const bgImage = await fill.evaluate((el) => getComputedStyle(el).backgroundImage);
  expect(bgImage).toContain('linear-gradient');
  expect(bgImage).toContain('rgb(29, 185, 84)');
  expect(bgImage).toContain('rgb(124, 179, 255)');
});

test('transcode detection toggle uses settings tab accent color when on', async ({ page }) => {
  await mockSettingsWithFlacalyzer(page, { analyzing: false, analyzed: 0, analysisTotal: 0, flacalyzerEnabled: true });

  await page.goto('/');
  await expect(page.locator('.bottom-nav')).toBeVisible({ timeout: 15_000 });
  await page.locator('.nav-tab').nth(2).click();

  const toggle = page.locator('.toggle.on').first();
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveCSS('background-color', 'rgb(124, 179, 255)');
});
