import { apiRequest } from '../api';
import { iconEl } from '../icons';
import type { LANQRStatus, PhoneSyncFolder } from '../types';
import { el, parseSafeSVG, showToast, statusBadge } from '../ui';

export function renderPhoneView(container: HTMLElement): () => void {
  const syncthingCard = el('div', { class: 'card' });
  const lanQrCard = el('div', { class: 'card' });
  const adbCard = el('div', { class: 'card' });

  const topGrid = el('div', { class: 'grid-2' }, syncthingCard, lanQrCard);
  container.replaceChildren(topGrid, adbCard);

  const loadSyncthing = async () => {
    try {
      const data = await apiRequest<{ folders: PhoneSyncFolder[]; syncthingId: string; syncthingQrSvg?: string }>('/api/v1/phone/folders');
      syncthingCard.replaceChildren();
      const scanAll = el('button', { type: 'button', class: 'btn-primary btn-sm' }, iconEl('refresh', 14), 'Scan All');
      scanAll.addEventListener('click', () => {
        void apiRequest('/api/v1/phone/syncthing/action', { method: 'POST', body: JSON.stringify({ action: 'scan' }) })
          .then((r: any) => showToast(r.offline ? `Syncthing daemon offline (${r.error})` : 'Syncthing folder scan triggered.'));
      });
      syncthingCard.append(
        el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Syncthing LAN-Only Two-Way Sync'), scanAll),
        el('p', { class: 'vcell-sub' }, `Device ID: ${data.syncthingId || 'Pending start'} · Global Discovery & Relays disabled`),
      );
      if (data.syncthingQrSvg) {
        const svgNode = parseSafeSVG(data.syncthingQrSvg);
        if (svgNode) syncthingCard.append(el('div', { class: 'qr-frame', style: 'margin:8px 0;' }, svgNode));
      }
      const list = el('div', { class: 'account-stack' });
      for (const f of data.folders ?? []) {
        const btn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('play', 14), 'Sync Now');
        btn.addEventListener('click', () => {
          void apiRequest('/api/v1/phone/syncthing/action', { method: 'POST', body: JSON.stringify({ action: 'scan', folderId: f.id }) })
            .then((r: any) => showToast(r.offline ? `Adapter offline: ${r.error}` : `Triggered sync for ${f.label}`));
        });
        list.append(
          el(
            'div',
            { class: 'account-card' },
            el(
              'div',
              { class: 'account-row' },
              el('strong', {}, f.label),
              f.ragNotify ? statusBadge('info', 'zap', 'RAG :27130 Live') : statusBadge('success', 'smartphone', f.transport),
            ),
            el('div', { class: 'account-row' }, el('span', { class: 'vcell-sub' }, `${f.localPath} ↔ ${f.remotePath}`), btn),
          ),
        );
      }
      syncthingCard.append(list);
    } catch (err) {
      syncthingCard.replaceChildren(el('div', { class: 'error-state' }, (err as Error).message));
    }
  };

  const loadLANQR = async () => {
    try {
      const st = await apiRequest<LANQRStatus>('/api/v1/lan-qr');
      lanQrCard.replaceChildren();
      const toggleBtn = el(
        'button',
        { type: 'button', class: st.enabled ? 'btn-danger btn-sm' : 'btn-primary btn-sm' },
        iconEl('qr', 14),
        st.enabled ? 'Disable Drop-Zone' : 'Enable QR Drop-Zone (:53317)',
      );
      toggleBtn.addEventListener('click', async () => {
        await apiRequest<LANQRStatus>('/api/v1/lan-qr', {
          method: 'POST',
          body: JSON.stringify({ enabled: !st.enabled }),
        });
        showToast(!st.enabled ? 'Ephemeral HTTPS TLS Drop-Zone enabled.' : 'LAN Drop-Zone stopped.');
        await loadLANQR();
      });

      lanQrCard.append(
        el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Browser QR Drop-Zone (OFF by default)'), toggleBtn),
        el('p', { class: 'vcell-sub' }, `Isolated HTTPS TLS port :${st.port} with 6-digit PIN · Auto-route: ${st.autoRoute ? 'ON' : 'OFF'}`),
      );
      if (st.enabled && st.pin) {
        lanQrCard.append(
          el('p', {}, el('strong', {}, `PIN: ${st.pin}`), el('span', { class: 'vcell-sub' }, ` · ${st.url ?? ''}`)),
        );
        if (st.qrSvg) {
          const svg = parseSafeSVG(st.qrSvg);
          if (svg) lanQrCard.append(el('div', { class: 'qr-frame' }, svg));
        }
      } else {
        lanQrCard.append(el('div', { class: 'empty-state', style: 'margin-top:12px;' }, iconEl('qr', 24), el('p', {}, 'Drop-Zone is currently disabled (zero open LAN ports).')));
      }
    } catch (err) {
      lanQrCard.replaceChildren(el('div', { class: 'error-state' }, (err as Error).message));
    }
  };

  const renderADB = () => {
    const outBox = el('div', { class: 'code-log' }, 'Click "Scan USB/Wireless ADB" or "Generate Wireless QR" to inspect MediaStore delta.');
    const scanBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('smartphone', 14), 'Scan ADB Devices');
    const deltaBtn = el('button', { type: 'button', class: 'btn-primary btn-sm' }, iconEl('search', 14), 'Inspect MediaStore Delta');
    const qrBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('qr', 14), 'Wireless ADB QR');
    const qrSlot = el('div', {});

    scanBtn.addEventListener('click', async () => {
      const res = await apiRequest<{ devices: any[]; offline?: boolean; error?: string }>('/api/v1/phone/adb/action', {
        method: 'POST',
        body: JSON.stringify({ action: 'devices' }),
      });
      outBox.textContent = res.offline ? `ADB offline: ${res.error}` : JSON.stringify(res.devices, null, 2);
    });
    deltaBtn.addEventListener('click', async () => {
      const res = await apiRequest<{ items: any[]; offline?: boolean; error?: string }>('/api/v1/phone/adb/action', {
        method: 'POST',
        body: JSON.stringify({ action: 'delta' }),
      });
      outBox.textContent = res.offline ? `ADB offline: ${res.error}` : JSON.stringify(res.items, null, 2);
    });
    qrBtn.addEventListener('click', async () => {
      const res = await apiRequest<{ payload: string; qrSvg: string }>('/api/v1/phone/adb/action', {
        method: 'POST',
        body: JSON.stringify({ action: 'qr' }),
      });
      const svg = parseSafeSVG(res.qrSvg);
      qrSlot.replaceChildren(svg ? el('div', { class: 'qr-frame' }, svg) : el('span', {}, res.payload));
      outBox.textContent = `Wireless ADB QR payload: ${res.payload}`;
    });

    adbCard.replaceChildren(
      el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Wireless QR & USB ADB MediaStore Diff'), el('div', { class: 'toolbar' }, scanBtn, deltaBtn, qrBtn)),
      qrSlot,
      outBox,
    );
  };

  void loadSyncthing();
  void loadLANQR();
  renderADB();
  return () => {};
}
