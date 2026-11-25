import { apiRequest } from '../api';
import { iconEl } from '../icons';
import type { AppSnapshot, DuplicateGroup, TelemetrySnapshot, Tombstone } from '../types';
import { confirmAction, el, formatBytes, showToast, statusBadge } from '../ui';

export function renderBackplaneView(container: HTMLElement, getLatestTelemetry: () => TelemetrySnapshot | null): () => void {
  const apiCard = el('div', { class: 'card' });
  const dupCard = el('div', { class: 'card' });
  const tombCard = el('div', { class: 'card' });
  const logsCard = el('div', { class: 'card' });

  container.replaceChildren(el('div', { class: 'grid-2' }, apiCard, dupCard), tombCard, logsCard);

  let logsVisible = false;

  const loadAll = async () => {
    try {
      const [snapsRes, dupsRes, tombsRes] = await Promise.all([
        apiRequest<{ snapshots: AppSnapshot[] }>('/api/v1/app-snapshots'),
        apiRequest<{ duplicates: DuplicateGroup[] }>('/api/v1/duplicates'),
        apiRequest<{ tombstones: Tombstone[] }>('/api/v1/tombstones'),
      ]);

      apiCard.replaceChildren(
        el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Loopback :59432 Backplane & Kopia'), statusBadge('success', 'terminal', 'Bearer + Origin Guard')),
        el('p', { class: 'vcell-sub' }, 'POST http://127.0.0.1:59432/api/v1/sync?app=<slug> · MCP: /mcp · VACUUM INTO + Kopia FastCDC'),
      );
      const snaps = snapsRes.snapshots ?? [];
      if (snaps.length === 0) {
        apiCard.append(el('div', { class: 'empty-state', style: 'margin-top:8px;' }, el('p', {}, 'No custom .exe app snapshots pushed yet.')));
      } else {
        const ul = el('ul', { class: 'account-stack' });
        for (const s of snaps.slice(0, 8)) {
          ul.append(el('li', { class: 'account-card' }, `${s.appSlug} · ${s.snapshotType} · ${s.relPath}${s.conflictPath ? ` (CONFLICT: ${s.conflictPath})` : ''}`));
        }
        apiCard.append(ul);
      }

      const dups = dupsRes.duplicates ?? [];
      dupCard.replaceChildren(el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Cross-Account Duplicate Inspector')));
      if (dups.length === 0) {
        dupCard.append(el('div', { class: 'empty-state' }, iconEl('check', 24), el('p', {}, 'Zero duplicate SHA-256 files across pooled accounts.')));
      } else {
        const list = el('div', { class: 'account-stack' });
        for (const d of dups) {
          list.append(
            el(
              'div',
              { class: 'account-card' },
              el('div', { class: 'account-row' }, el('strong', {}, `${d.entries[0]?.name ?? d.sha256.slice(0, 12)} (${d.count}x)`), statusBadge('warning', 'warn', `Wasted ${formatBytes(d.wastedBytes)}`)),
              el('div', { class: 'vcell-sub' }, d.entries.map((e) => `${e.provider}:${e.relPath}`).join(' · ')),
            ),
          );
        }
        dupCard.append(list);
      }

      const tombs = tombsRes.tombstones ?? [];
      tombCard.replaceChildren(el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, '30-Day Quarantine & Tombstone Review Queue')));
      if (tombs.length === 0) {
        tombCard.append(el('div', { class: 'empty-state' }, el('p', {}, 'No pending deletions or remote tombstones to review.')));
      } else {
        const tbl = el('table', { class: 'data-table' });
        tbl.append(el('thead', {}, el('tr', {}, el('th', { scope: 'col' }, 'Remote'), el('th', { scope: 'col' }, 'Path'), el('th', { scope: 'col' }, 'Resolution'), el('th', { scope: 'col' }, 'Actions'))));
        const tb = el('tbody', {});
        for (const t of tombs) {
          const restBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('undo', 14), 'Restore');
          restBtn.addEventListener('click', async () => {
            await apiRequest('/api/v1/tombstones/resolve', { method: 'POST', body: JSON.stringify({ id: t.id, resolution: 'RESTORED' }) });
            showToast(`Restored ${t.remotePath}`);
            await loadAll();
          });
          const purgeBtn = el('button', { type: 'button', class: 'btn-danger btn-sm' }, 'Confirm Purge');
          purgeBtn.addEventListener('click', async () => {
            if (await confirmAction('Confirm Permanent Purge?', `Mark ${t.remotePath} as permanently purged?`, 'Purge')) {
              await apiRequest('/api/v1/tombstones/resolve', { method: 'POST', body: JSON.stringify({ id: t.id, resolution: 'PURGED' }) });
              await loadAll();
            }
          });
          tb.append(el('tr', {}, el('td', {}, t.remoteId), el('td', {}, t.remotePath), el('td', {}, t.resolution), el('td', {}, restBtn, ' ', purgeBtn)));
        }
        tbl.append(tb);
        tombCard.append(tbl);
      }
    } catch (err) {
      apiCard.replaceChildren(el('div', { class: 'error-state' }, (err as Error).message));
    }
  };

  const toggleLogs = el('button', { type: 'button', class: 'btn-secondary btn-sm', 'aria-expanded': 'false' }, 'Show Live Redacted Logs');
  const logBox = el('pre', { class: 'code-log', hidden: '' }, 'No events captured yet.');
  toggleLogs.addEventListener('click', () => {
    logsVisible = !logsVisible;
    logBox.hidden = !logsVisible;
    toggleLogs.textContent = logsVisible ? 'Hide Live Redacted Logs' : 'Show Live Redacted Logs';
    toggleLogs.setAttribute('aria-expanded', String(logsVisible));
    const snap = getLatestTelemetry();
    if (snap?.recentEvents?.length) {
      logBox.textContent = snap.recentEvents.map((e) => `[${e.ts}] (${e.component}) ${e.message}`).join('\n');
    }
  });
  logsCard.append(el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Redacted Backplane & Telemetry Stream'), toggleLogs), logBox);

  void loadAll();
  return () => {};
}
