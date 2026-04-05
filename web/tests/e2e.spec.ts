import { test, expect, type Page } from '@playwright/test';

// Tests require Go backend running on port 8080 with SWIPEFI_MUSIC_DIR
// pointing at the FLAC folder. "Test - Flacalyzer" should be at the root.

test.describe.serial('SwipeFi E2E', () => {
  // Navigate to folder view, handling the case where player may already be active
  async function ensureFolderView(page: Page) {
    await page.goto('/');

    const folderNav = page.locator('.folder-nav');
    const nowPlaying = page.locator('.now-playing');
    const setup = page.locator('.center-screen');

    await expect(folderNav.or(nowPlaying).or(setup)).toBeVisible({ timeout: 15_000 });

    if (await nowPlaying.isVisible()) {
      await page.locator('.back-btn').click();
      await expect(folderNav).toBeVisible({ timeout: 5_000 });
    } else if (await setup.isVisible()) {
      // Need to select device first — click the first device button if any
      const deviceBtn = page.locator('.device-btn').first();
      if (await deviceBtn.isVisible()) {
        await deviceBtn.click();
        await expect(folderNav).toBeVisible({ timeout: 10_000 });
      }
    }
  }

  test('shows folder navigation with test folder', async ({ page }) => {
    await ensureFolderView(page);
    await expect(page.getByText('Test - Flacalyzer')).toBeVisible({ timeout: 5_000 });
  });

  test('can navigate into a folder and back', async ({ page }) => {
    await ensureFolderView(page);

    const firstFolder = page.locator('.folder-link').first();
    const folderName = await firstFolder.locator('.folder-name').textContent();
    await firstFolder.click();

    await expect(page.locator('.breadcrumbs')).toContainText(folderName!);

    // Navigate back via Home
    await page.locator('.crumb').first().click();
    await expect(page.getByText('Test - Flacalyzer')).toBeVisible();
  });

  test('sort selector works', async ({ page }) => {
    await ensureFolderView(page);

    const sortSelect = page.locator('.sort-select');
    await expect(sortSelect).toBeVisible();
    await expect(sortSelect).toHaveValue('added_at:desc');

    await sortSelect.selectOption('play_count:asc');
    await expect(sortSelect).toHaveValue('play_count:asc');
  });

  test('play button starts playback on renderer', async ({ page }) => {
    await ensureFolderView(page);

    const testFolder = page.locator('.folder-item', { hasText: 'Test - Flacalyzer' });
    await testFolder.locator('.play-btn').click();

    await expect(page.locator('.now-playing')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('.swipe-card')).toBeVisible();
    await expect(page.locator('.title')).not.toBeEmpty();
    await expect(page.locator('.transport')).toBeVisible();
    await expect(page.locator('.queue-info')).toContainText('/');
  });

  test('pause and resume work', async ({ page }) => {
    await ensureFolderView(page);

    // Start playback
    const testFolder = page.locator('.folder-item', { hasText: 'Test - Flacalyzer' });
    await testFolder.locator('.play-btn').click();
    await expect(page.locator('.swipe-card')).toBeVisible({ timeout: 5_000 });
    await page.waitForTimeout(2000);

    // Pause
    await page.locator('.play-pause-btn').click();
    await page.waitForTimeout(1000);

    const paused = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );
    expect(paused.state).toBe('paused');

    // Resume
    await page.locator('.play-pause-btn').click();
    await page.waitForTimeout(1000);

    const resumed = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );
    expect(resumed.state).toBe('playing');
  });

  test('next button advances to a different track', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.now-playing').or(page.locator('.folder-nav'))).toBeVisible({ timeout: 15_000 });

    const before = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );

    const after = await page.evaluate(() =>
      fetch('/api/player/next', { method: 'POST' }).then(r => r.json())
    );

    expect(after.track?.id).not.toBe(before.track?.id);
  });

  test('swipe right (keep) advances track via API', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.now-playing').or(page.locator('.folder-nav'))).toBeVisible({ timeout: 15_000 });

    const before = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );

    const after = await page.evaluate(() =>
      fetch('/api/player/next', { method: 'POST' }).then(r => r.json())
    );

    expect(after.track?.id).not.toBe(before.track?.id);
  });

  test('swipe left (reject) moves file to to_delete', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.now-playing').or(page.locator('.folder-nav'))).toBeVisible({ timeout: 15_000 });

    const before = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );
    const rejectedPath = before.track?.path;
    expect(rejectedPath).toBeTruthy();

    const after = await page.evaluate(() =>
      fetch('/api/player/reject', { method: 'POST' }).then(r => r.json())
    );

    expect(after.track?.path).not.toBe(rejectedPath);

    const tracks: any[] = await page.evaluate(() =>
      fetch('/api/tracks?folder=Test%20-%20Flacalyzer').then(r => r.json())
    );
    const found = tracks.find(t => t.path === rejectedPath);
    expect(found).toBeUndefined();
  });

  test('back button shows folder view with mini player', async ({ page }) => {
    await page.goto('/');
    const nowPlaying = page.locator('.now-playing');
    const folderNav = page.locator('.folder-nav');

    await expect(nowPlaying.or(folderNav)).toBeVisible({ timeout: 15_000 });

    // If we're on folder view (player idle), start playback first
    if (await folderNav.isVisible()) {
      const testFolder = page.locator('.folder-item', { hasText: 'Test - Flacalyzer' });
      await testFolder.locator('.play-btn').click();
      await expect(nowPlaying).toBeVisible({ timeout: 5_000 });
    }

    // Press back
    await page.locator('.back-btn').click();
    await expect(folderNav).toBeVisible();

    // Mini player should be visible
    await expect(page.locator('.mini-player')).toBeVisible();
    await expect(page.locator('.mini-title')).not.toBeEmpty();

    // Click mini player to return
    await page.locator('.mini-player').click();
    await expect(nowPlaying).toBeVisible();
  });
});
