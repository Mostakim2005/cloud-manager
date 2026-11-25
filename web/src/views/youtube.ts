import { apiRequest } from '../api';
import { iconEl } from '../icons';
import type { Job, YouTubeQuota } from '../types';
import { el, formatBytes, showToast, statusBadge } from '../ui';

export function renderYouTubeView(container: HTMLElement): () => void {
  const quotaCard = el('div', { class: 'card' });
  const queueCard = el('div', { class: 'card' });
  container.replaceChildren(quotaCard, queueCard);

  const loadYouTube = async () => {
    try {
      const [quota, jobsRes] = await Promise.all([
        apiRequest<YouTubeQuota>('/api/v1/youtube/quota'),
        apiRequest<{ jobs: Job[] }>('/api/v1/jobs'),
      ]);

      const pct = Math.min(100, Math.round((quota.unitsUsed / quota.dailyCap) * 100));
      quotaCard.replaceChildren(
        el(
          'div',
          { class: 'card-header' },
          el('h2', { class: 'card-title' }, 'YouTube Private Video Pipeline (16 MB Chunks)'),
          statusBadge(quota.canUploadNext ? 'success' : 'warning', 'youtube', 'Forced: privacyStatus = private'),
        ),
        el('p', {}, `PT Quota (${quota.ptDate}): ${quota.unitsUsed.toLocaleString()} / ${quota.dailyCap.toLocaleString()} safety cap (10,000 Google ceiling)`),
        el('div', { class: 'meter', style: 'margin:8px 0;' }, el('div', { class: `meter-fill${pct > 80 ? ' warn' : ''}`, style: `width:${pct}%` })),
        el('p', { class: 'vcell-sub' }, `Remaining today: ${quota.remaining.toLocaleString()} units (1,650/video) · Next PT reset: ${quota.nextResetPt}`),
      );

      const srcIn = el('input', { type: 'text', class: 'input', placeholder: 'Local video path (e.g. D:\\Videos\\demo.mp4)', required: '' }) as HTMLInputElement;
      const plIn = el('input', { type: 'text', class: 'input', placeholder: 'Playlist ID (optional)' }) as HTMLInputElement;
      const dryBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, 'Dry-Run');
      const qBtn = el('button', { type: 'submit', class: 'btn-primary btn-sm' }, iconEl('youtube', 14), 'Queue Private Upload');
      const form = el('form', { class: 'toolbar' }, srcIn, plIn, dryBtn, qBtn);

      dryBtn.addEventListener('click', async () => {
        if (!srcIn.value.trim()) return;
        const r = await apiRequest<any>('/api/v1/youtube/queue', {
          method: 'POST',
          body: JSON.stringify({ sourcePath: srcIn.value.trim(), playlistId: plIn.value.trim(), dryRun: true }),
        });
        showToast(`Dry-Run: privacy=${r.privacyStatus}, willDefer=${r.willDefer}`);
      });

      form.addEventListener('submit', async (e) => {
        e.preventDefault();
        try {
          await apiRequest('/api/v1/youtube/queue', {
            method: 'POST',
            body: JSON.stringify({ sourcePath: srcIn.value.trim(), playlistId: plIn.value.trim(), dryRun: false }),
          });
          showToast('Video queued for private 16 MB resumable upload.');
          srcIn.value = '';
          await loadYouTube();
        } catch (err) {
          showToast(`Queue failed: ${(err as Error).message}`);
        }
      });

      const ytJobs = (jobsRes.jobs ?? []).filter((j) => j.kind === 'youtube_upload');
      queueCard.replaceChildren(
        el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Upload Queue & Resumable Sessions')),
        form,
      );
      if (ytJobs.length === 0) {
        queueCard.append(el('div', { class: 'empty-state' }, iconEl('youtube', 24), el('p', {}, 'No YouTube upload jobs queued yet.')));
      } else {
        const tbl = el('table', { class: 'data-table' });
        tbl.append(el('thead', {}, el('tr', {}, el('th', { scope: 'col' }, 'Source'), el('th', { scope: 'col' }, 'Progress'), el('th', { scope: 'col' }, 'Status'))));
        const tb = el('tbody', {});
        for (const j of ytJobs) {
          tb.append(el('tr', {}, el('td', {}, j.sourcePath), el('td', {}, `${formatBytes(j.bytesDone)} / ${formatBytes(j.bytesTotal)}`), el('td', {}, statusBadge('info', 'activity', j.status))));
        }
        tbl.append(tb);
        queueCard.append(tbl);
      }
    } catch (err) {
      quotaCard.replaceChildren(el('div', { class: 'error-state' }, (err as Error).message));
    }
  };

  void loadYouTube();
  return () => {};
}
