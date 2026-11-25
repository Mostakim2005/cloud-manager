import { iconEl, type IconName } from './icons';

export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Record<string, string> = {},
  ...children: (Node | string)[]
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    node.setAttribute(k, v);
  }
  for (const child of children) {
    node.append(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return node;
}

export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let val = bytes;
  let i = 0;
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024;
    i++;
  }
  return `${val >= 10 || i === 0 ? val.toFixed(0) : val.toFixed(1)} ${units[i]}`;
}

export function statusBadge(kind: 'success' | 'warning' | 'danger' | 'info', icon: IconName, text: string): HTMLElement {
  return el('span', { class: `status-badge status-${kind}` }, iconEl(icon, 14), el('span', {}, text));
}

export function parseSafeSVG(svgString: string): SVGElement | null {
  if (!svgString || !svgString.trim().startsWith('<svg')) {
    return null;
  }
  const doc = new DOMParser().parseFromString(svgString, 'image/svg+xml');
  const svg = doc.documentElement;
  if (svg.nodeName.toLowerCase() !== 'svg') {
    return null;
  }
  if (svg.querySelector('script, foreignObject')) {
    return null;
  }
  return document.importNode(svg, true) as unknown as SVGElement;
}

export function showToast(message: string, action?: { label: string; onClick: () => void }): void {
  const region = document.getElementById('toast-region');
  if (!region) {
    return;
  }
  const toast = el('div', { class: 'toast' }, el('span', {}, message));
  if (action) {
    const actBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, action.label);
    actBtn.addEventListener('click', () => {
      action.onClick();
      toast.remove();
    });
    toast.append(actBtn);
  }
  const closeBtn = el('button', { type: 'button', class: 'btn-ghost btn-sm', 'aria-label': 'Dismiss notification' }, '×');
  closeBtn.addEventListener('click', () => toast.remove());
  toast.append(closeBtn);
  region.append(toast);

  // Actionable/error toasts persist until dismissed (WCAG 2.2.2); passive toasts auto-clear.
  if (!action) {
    setTimeout(() => toast.remove(), 4500);
  }
}

export function openModalDialog(dialog: HTMLDialogElement): void {
  dialog.returnValue = '';
  try {
    if (typeof dialog.showModal === 'function') {
      dialog.showModal();
      return;
    }
  } catch {
    // Fallback for test environments where HTMLDialogElement.showModal throws or is unimplemented.
  }
  dialog.setAttribute('open', '');
}

export function confirmAction(title: string, message: string, confirmLabel = 'Confirm'): Promise<boolean> {
  const dialog = document.getElementById('confirm-dialog') as HTMLDialogElement | null;
  const titleEl = document.getElementById('confirm-title');
  const msgEl = document.getElementById('confirm-message');
  const okBtn = document.getElementById('confirm-ok-btn');
  if (!dialog || !titleEl || !msgEl || !okBtn) {
    return Promise.resolve(false);
  }
  titleEl.textContent = title;
  msgEl.textContent = message;
  okBtn.textContent = confirmLabel;

  return new Promise((resolve) => {
    const onClose = () => {
      const accepted = dialog.returnValue === 'confirm';
      dialog.returnValue = '';
      dialog.removeEventListener('close', onClose);
      resolve(accepted);
    };
    dialog.addEventListener('close', onClose);
    openModalDialog(dialog);
  });
}

export function showQuickLookModal(title: string, content: Node): void {
  const dialog = document.getElementById('quick-look-dialog') as HTMLDialogElement | null;
  const titleEl = document.getElementById('ql-title');
  const bodyEl = document.getElementById('ql-body');
  if (!dialog || !titleEl || !bodyEl) {
    return;
  }
  titleEl.textContent = title;
  bodyEl.replaceChildren(content);
  openModalDialog(dialog);
}
