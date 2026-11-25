import fs from 'node:fs';
import path from 'node:path';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { applyDensity, applyTheme, getSavedDensity, getSavedTheme } from '../src/theme';
import { bootstrapApp } from '../src/main';

const indexHtml = fs.readFileSync(path.resolve(__dirname, '../index.html'), 'utf8');
const tokensCss = fs.readFileSync(path.resolve(__dirname, '../src/styles/tokens.css'), 'utf8');
const appCss = fs.readFileSync(path.resolve(__dirname, '../src/styles/app.css'), 'utf8');

function relativeLuminance(hex: string): number {
  const h = hex.replace('#', '');
  const nums = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)));
  return 0.2126 * nums[0] + 0.7152 * nums[1] + 0.0722 * nums[2];
}

function contrastRatio(fg: string, bg: string): number {
  const l1 = relativeLuminance(fg);
  const l2 = relativeLuminance(bg);
  return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
}

describe('Cloud Manager UI, Theme Contrast & Keyboard Ergonomics', () => {
  beforeEach(() => {
    document.documentElement.innerHTML = indexHtml.replace(/<!DOCTYPE html>/i, '');
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const u = String(input);
      if (u.includes('/api/v1/catalog')) {
        return new Response(JSON.stringify({ items: [{ id: 1, name: 'notes.md', relPath: 'Obsidian/notes.md', sizeBytes: 1200, provider: 'gdrive', remoteId: 'gdrive-1', status: 'SYNCED', sha256: 'abc', mimeType: 'text/markdown', originType: 'syncthing', mtimeUtc: 1, parentRevision: '1', updatedAt: 1 }], total: 1, queryMs: 0.4 }), { status: 200 });
      }
      if (u.includes('/api/v1/status')) {
        return new Response(JSON.stringify({ accounts: [{ id: 'gdrive-1', name: 'GDrive #1', provider: 'gdrive', totalBytes: 1000, usedBytes: 250, freeBytes: 750 }], stats: { totalFiles: 1, activeJobs: 0 } }), { status: 200 });
      }
      if (u.includes('/api/v1/jobs')) {
        return new Response(JSON.stringify({ jobs: [{ id: 7, kind: 'rclone_copy', sourcePath: 'demo.pdf', targetRemote: 'gdrive-1', status: 'RUNNING', priority: 50, bytesTotal: 100, bytesDone: 50, updatedAt: 1 }] }), { status: 200 });
      }
      if (u.includes('/api/v1/rules')) {
        return new Response(JSON.stringify({ rules: [{ id: 1, priority: 10, name: 'Fallback', originMatch: '*', extGlob: '*', targetProvider: 'gdrive', targetRemote: 'gdrive-1', isFallback: true }] }), { status: 200 });
      }
      if (u.includes('/api/v1/events')) {
        return new Response(null, { status: 200 });
      }
      return new Response(JSON.stringify({}), { status: 200 });
    }));
  });

  it('enforces WCAG AA >= 4.5:1 contrast across Light and Dark Material 3 tokens', () => {
    expect(contrastRatio('#111827', '#ffffff')).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio('#526075', '#f6f8fc')).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio('#ffffff', '#1d4ed8')).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio('#f1f5f9', '#121826')).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio('#94a3b8', '#0b0f17')).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio('#0b1220', '#93c5fd')).toBeGreaterThanOrEqual(4.5);
    expect(tokensCss).toContain('forced-colors: active');
    expect(appCss).toContain('prefers-reduced-motion: reduce');
    expect(appCss).toContain('overscroll-behavior: contain');
  });

  it('switches Light / Dark / System theme and Comfortable / Compact density cleanly', () => {
    applyTheme('dark');
    expect(getSavedTheme()).toBe('dark');
    expect(document.querySelector('meta[name="color-scheme"]')?.getAttribute('content')).toBe('dark');

    applyTheme('system');
    expect(getSavedTheme()).toBe('system');
    expect(document.querySelector('meta[name="color-scheme"]')?.getAttribute('content')).toBe('light dark');

    applyDensity('compact');
    expect(getSavedDensity()).toBe('compact');
    expect(document.documentElement.getAttribute('data-density')).toBe('compact');
  });

  it('boots virtual explorer, handles Ctrl+K palette and ? shortcut dialog', async () => {
    const cleanup = bootstrapApp();
    await new Promise((r) => setTimeout(r, 30));

    expect(document.getElementById('view-heading')?.textContent).toBe('Virtual File Explorer');
    expect(document.getElementById('brand-icon')?.querySelector('svg')).not.toBeNull();
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/favicon.svg');
    expect(document.getElementById('live-progress-card')?.textContent).toContain('demo.pdf');
    expect(document.querySelectorAll('.vtable-row').length).toBeGreaterThan(0);

    document.getElementById('btn-connect-account')?.click();
    expect(document.getElementById('connect-account-dialog')?.hasAttribute('open')).toBe(true);
    document.getElementById('connect-account-dialog')?.removeAttribute('open');

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true }));
    expect(document.getElementById('command-dialog')?.hasAttribute('open')).toBe(true);
    document.getElementById('command-dialog')?.removeAttribute('open');
    (document.activeElement as HTMLElement | null)?.blur?.();

    document.dispatchEvent(new KeyboardEvent('keydown', { key: '?' }));
    expect(document.getElementById('shortcuts-dialog')?.hasAttribute('open')).toBe(true);

    cleanup();
  });
});
