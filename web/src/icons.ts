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
  return createElement(ICONS[name], {
    width: size,
    height: size,
    'stroke-width': '1.75',
    'aria-hidden': 'true',
  });
}
