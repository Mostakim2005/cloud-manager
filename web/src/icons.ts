import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Cloud,
  Copy,
  Eye,
  Folder,
  HelpCircle,
  Menu,
  Pause,
  Play,
  QrCode,
  RefreshCw,
  RotateCcw,
  Search,
  Settings,
  Smartphone,
  Terminal,
  Trash2,
  XCircle,
  Youtube,
  Zap,
  createElement,
  type IconNode,
} from 'lucide';

export type IconName =
  | 'cloud'
  | 'folder'
  | 'zap'
  | 'smartphone'
  | 'youtube'
  | 'terminal'
  | 'settings'
  | 'help'
  | 'check'
  | 'warn'
  | 'error'
  | 'activity'
  | 'search'
  | 'eye'
  | 'trash'
  | 'menu'
  | 'refresh'
  | 'qr'
  | 'play'
  | 'pause'
  | 'undo'
  | 'copy';

const ICONS: Record<IconName, IconNode> = {
  cloud: Cloud,
  folder: Folder,
  zap: Zap,
  smartphone: Smartphone,
  youtube: Youtube,
  terminal: Terminal,
  settings: Settings,
  help: HelpCircle,
  check: CheckCircle2,
  warn: AlertTriangle,
  error: XCircle,
  activity: Activity,
  search: Search,
  eye: Eye,
  trash: Trash2,
  menu: Menu,
  refresh: RefreshCw,
  qr: QrCode,
  play: Play,
  pause: Pause,
  undo: RotateCcw,
  copy: Copy,
};

export function iconEl(name: IconName, size = 16): SVGElement {
  const svg = createElement(ICONS[name]);
  svg.setAttribute('width', String(size));
  svg.setAttribute('height', String(size));
  svg.setAttribute('stroke-width', '1.75');
  svg.setAttribute('aria-hidden', 'true');
  return svg;
}

export function brandLogoEl(size = 24): SVGElement {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 64 64');
  svg.setAttribute('width', String(size));
  svg.setAttribute('height', String(size));
  svg.setAttribute('aria-hidden', 'true');

  const defs = document.createElementNS(ns, 'defs');
  const grad = document.createElementNS(ns, 'linearGradient');
  grad.setAttribute('id', 'cm-brand-grad');
  grad.setAttribute('x1', '0%');
  grad.setAttribute('y1', '100%');
  grad.setAttribute('x2', '100%');
  grad.setAttribute('y2', '0%');
  const s1 = document.createElementNS(ns, 'stop');
  s1.setAttribute('offset', '0%');
  s1.setAttribute('stop-color', '#1d4ed8');
  const s2 = document.createElementNS(ns, 'stop');
  s2.setAttribute('offset', '100%');
  s2.setAttribute('stop-color', '#0ea5e9');
  grad.append(s1, s2);
  defs.append(grad);

  const badge = document.createElementNS(ns, 'rect');
  badge.setAttribute('x', '3.2');
  badge.setAttribute('y', '3.2');
  badge.setAttribute('width', '57.6');
  badge.setAttribute('height', '57.6');
  badge.setAttribute('rx', '13');
  badge.setAttribute('fill', 'url(#cm-brand-grad)');

  const cloud = document.createElementNS(ns, 'path');
  cloud.setAttribute(
    'd',
    'M20.5 45.4h23c4 0 7.1-3.1 7.1-7.1 0-3.7-2.8-6.7-6.4-7-1.1-6.2-6.5-10.9-13.1-10.9-6.2 0-11.4 4.2-12.9 10-3.6.7-6.2 3.8-6.2 7.5 0 4.2 3.3 7.5 8.5 7.5z',
  );
  cloud.setAttribute('fill', '#ffffff');

  const upArrow = document.createElementNS(ns, 'path');
  upArrow.setAttribute('d', 'M26.9 40.3V31.4m0 0l-3.5 3.5m3.5-3.5l3.5 3.5');
  upArrow.setAttribute('fill', 'none');
  upArrow.setAttribute('stroke', '#1d4ed8');
  upArrow.setAttribute('stroke-width', '3.2');
  upArrow.setAttribute('stroke-linecap', 'round');
  upArrow.setAttribute('stroke-linejoin', 'round');

  const downArrow = document.createElementNS(ns, 'path');
  downArrow.setAttribute('d', 'M37.1 30.1V39m0 0l-3.5-3.5m3.5 3.5l3.5-3.5');
  downArrow.setAttribute('fill', 'none');
  downArrow.setAttribute('stroke', '#0284c7');
  downArrow.setAttribute('stroke-width', '3.2');
  downArrow.setAttribute('stroke-linecap', 'round');
  downArrow.setAttribute('stroke-linejoin', 'round');

  svg.append(defs, badge, cloud, upArrow, downArrow);
  return svg;
}
