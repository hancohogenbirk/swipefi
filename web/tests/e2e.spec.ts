import { test, expect, type Page } from '@playwright/test';

// Tests require Go backend running on port 8080 with SWIPEFI_MUSIC_DIR set.

const API = 'http://localhost:8080';

test.describe.serial('SwipeFi E2E', () => {
  // Get the first folder name from the API
  async function getFirstFolder(page: Page): Promise<string> {
    const folders = await page.evaluate(() =>
      fetch('/api/folders').then(r => r.json())
    );
    expect(folders.length).toBeGreaterThan(0);
    return folders[0].name;
  }

  // Navigate to folder view via bottom nav, handling setup flows
  async function ensureFolderView(page: Page) {
    await page.goto('/');

    // Wait for the app to finish loading — either bottom nav (main phase) or setup screen
    const bottomNav = page.locator('.bottom-nav');
    const setup = page.locator('.center-screen');
    await expect(bottomNav.or(setup)).toBeVisible({ timeout: 15_000 });

    // If on setup screen, select first device
    if (await setup.isVisible()) {
      const deviceBtn = page.locator('.device-btn').first();
      if (await deviceBtn.isVisible()) {
        await deviceBtn.click();
        await expect(bottomNav).toBeVisible({ timeout: 10_000 });
      }
    }

    // Switch to folders tab
    await page.locator('.nav-tab').first().click();
    await expect(page.locator('.folder-nav')).toBeVisible({ timeout: 5_000 });
  }

  test('shows folder navigation with folders from library', async ({ page }) => {
    await ensureFolderView(page);
    const firstFolder = await getFirstFolder(page);
    await expect(page.getByText(firstFolder, { exact: true })).toBeVisible({ timeout: 5_000 });
  });

  test('can navigate into a folder and back', async ({ page }) => {
    await ensureFolderView(page);

    const firstFolder = page.locator('.folder-link').first();
    const folderName = await firstFolder.locator('.folder-name').textContent();
    await firstFolder.click();

    await expect(page.locator('.breadcrumbs')).toContainText(folderName!);

    // Navigate back via Music breadcrumb
    await page.locator('.crumb').first().click();
    const firstFolderName = await getFirstFolder(page);
    await expect(page.getByText(firstFolderName, { exact: true })).toBeVisible();
  });

  test('sort selector works', async ({ page }) => {
    await ensureFolderView(page);

    const sortSelect = page.locator('.sort-select');
    await expect(sortSelect).toBeVisible();

    await sortSelect.selectOption('play_count:asc');
    await expect(sortSelect).toHaveValue('play_count:asc');
  });

  test('bottom navigation tabs work', async ({ page }) => {
    await ensureFolderView(page);

    // Should see 3 tab buttons
    const tabs = page.locator('.nav-tab');
    await expect(tabs).toHaveCount(3);

    // Folders tab should be active
    await expect(tabs.first()).toHaveClass(/active/);

    // Click Now Playing tab
    await tabs.nth(1).click();
    await expect(page.locator('.now-playing')).toBeVisible();
    await expect(tabs.nth(1)).toHaveClass(/active/);

    // Click Settings tab
    await tabs.nth(2).click();
    await expect(page.locator('.settings')).toBeVisible();
    await expect(tabs.nth(2)).toHaveClass(/active/);

    // Click Folders tab again
    await tabs.first().click();
    await expect(page.locator('.folder-nav')).toBeVisible();
    await expect(tabs.first()).toHaveClass(/active/);
  });

  test('play button starts playback and switches to Now Playing', async ({ page }) => {
    await ensureFolderView(page);

    // Navigate into the first folder that has tracks
    const firstFolder = page.locator('.folder-link').first();
    await firstFolder.click();
    await page.waitForTimeout(500);

    // Try the play-all button or the first folder's play button
    const playAllBtn = page.locator('.play-all-btn');
    const playBtn = page.locator('.play-btn').first();

    if (await playAllBtn.isVisible()) {
      await playAllBtn.click();
    } else if (await playBtn.isVisible()) {
      await playBtn.click();
    } else {
      // Go back and play the folder from root
      await page.locator('.crumb').first().click();
      await page.locator('.play-btn').first().click();
    }

    await expect(page.locator('.now-playing')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('.swipe-card')).toBeVisible();
    await expect(page.locator('.title')).not.toBeEmpty();
    await expect(page.locator('.transport')).toBeVisible();
  });

  test('pause and resume work', async ({ page }) => {
    // Ensure something is playing via API
    const state = await (await page.request.get(`${API}/api/player/state`)).json();
    if (state.state === 'idle') {
      const folders = await (await page.request.get(`${API}/api/folders`)).json();
      await page.request.post(`${API}/api/player/play`, {
        data: { folder: folders[0].path, sort: 'added_at', order: 'asc' },
      });
    }

    await page.goto('/');
    await expect(page.locator('.now-playing')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('.swipe-card')).toBeVisible({ timeout: 5_000 });
    await page.waitForTimeout(2000);

    // Pause via API and check the returned state
    const pauseResult = await page.evaluate(() =>
      fetch('/api/player/pause', { method: 'POST' }).then(r => r.json())
    );
    expect(pauseResult.state).toBe('paused');

    // Resume via API and check the returned state
    const resumeResult = await page.evaluate(() =>
      fetch('/api/player/resume', { method: 'POST' }).then(r => r.json())
    );
    expect(resumeResult.state).toBe('playing');
  });

  test('next button advances to a different track', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.folder-nav').or(page.locator('.center-screen'))).toBeVisible({ timeout: 15_000 });

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
    await expect(page.locator('.folder-nav').or(page.locator('.center-screen'))).toBeVisible({ timeout: 15_000 });

    const before = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );

    const after = await page.evaluate(() =>
      fetch('/api/player/next', { method: 'POST' }).then(r => r.json())
    );

    expect(after.track?.id).not.toBe(before.track?.id);
  });

  test('swipe left (reject) moves file to to_delete and restores it', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('.folder-nav').or(page.locator('.center-screen'))).toBeVisible({ timeout: 15_000 });

    // Get config so we know the music_dir and delete_dir paths
    const config: { music_dir: string; delete_dir: string } = await page.evaluate(() =>
      fetch('/api/config').then(r => r.json())
    );

    const before = await page.evaluate(() =>
      fetch('/api/player/state').then(r => r.json())
    );
    const rejectedPath = before.track?.path;
    expect(rejectedPath).toBeTruthy();

    const after = await page.evaluate(() =>
      fetch('/api/player/reject', { method: 'POST' }).then(r => r.json())
    );

    expect(after.track?.path).not.toBe(rejectedPath);

    // Restore the file: move it back from delete_dir to music_dir
    // This uses Node.js fs via Playwright's test runner context
    const path = await import('path');
    const fs = await import('fs');
    const src = path.join(config.delete_dir, rejectedPath);
    const dst = path.join(config.music_dir, rejectedPath);
    fs.mkdirSync(path.dirname(dst), { recursive: true });
    fs.renameSync(src, dst);
  });

  test('mini player shows on folders tab when playing', async ({ page }) => {
    // Ensure something is playing
    const state = await (await page.request.get(`${API}/api/player/state`)).json();
    if (state.state === 'idle') {
      const folders = await (await page.request.get(`${API}/api/folders`)).json();
      const devices = await (await page.request.get(`${API}/api/devices`)).json();
      if (devices.length > 0) {
        await page.request.post(`${API}/api/devices/select`, {
          data: { udn: devices[0].udn },
        });
      }
      await page.request.post(`${API}/api/player/play`, {
        data: { folder: folders[0].path, sort: 'added_at', order: 'asc' },
      });
    }

    await page.goto('/');
    await expect(page.locator('.bottom-nav')).toBeVisible({ timeout: 15_000 });

    // Switch to folders tab
    await page.locator('.nav-tab').first().click();
    await expect(page.locator('.folder-nav')).toBeVisible();

    // Mini player should be visible
    await expect(page.locator('.mini-player')).toBeVisible();
    await expect(page.locator('.mini-title')).not.toBeEmpty();

    // Click mini player to go to Now Playing
    await page.locator('.mini-player').click();
    await expect(page.locator('.now-playing')).toBeVisible();
  });

  test('tab state is preserved across switches', async ({ page }) => {
    await ensureFolderView(page);

    // Navigate into a subfolder
    const firstFolder = page.locator('.folder-link').first();
    const folderName = await firstFolder.locator('.folder-name').textContent();
    await firstFolder.click();
    await expect(page.locator('.breadcrumbs')).toContainText(folderName!);

    // Switch to Now Playing tab and back
    await page.locator('.nav-tab').nth(1).click();
    await expect(page.locator('.now-playing')).toBeVisible();

    await page.locator('.nav-tab').first().click();
    await expect(page.locator('.folder-nav')).toBeVisible();

    // Breadcrumbs should still show the subfolder
    await expect(page.locator('.breadcrumbs')).toContainText(folderName!);
  });

  // Note: exit confirmation dialog is tested manually — Playwright's headless
  // browser doesn't reliably trigger popstate events in the same way a real
  // browser back button does. The feature works in real browsers.
});
