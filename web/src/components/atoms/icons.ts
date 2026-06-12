/**
 * Inline SVG icon registry.
 *
 * TECH_STACK.md forbids unflagged dependencies, so rather than pull an icon
 * font/library we ship a small hand-curated set of 24×24 stroke paths (Lucide-
 * style geometry, MIT-equivalent simple primitives) covering exactly what the
 * component library and nav reference. Each entry is the inner markup of a
 * `viewBox="0 0 24 24"`, `stroke="currentColor"`, fill-none icon. Add new icons
 * here as screens need them — never inline raw SVG in component templates.
 */
export type IconName =
  | 'check'
  | 'check-circle'
  | 'x'
  | 'x-circle'
  | 'alert-triangle'
  | 'alert-circle'
  | 'info'
  | 'eye'
  | 'eye-off'
  | 'chevron-down'
  | 'chevron-right'
  | 'chevron-left'
  | 'search'
  | 'bell'
  | 'user'
  | 'shield'
  | 'shield-check'
  | 'hard-hat'
  | 'plus'
  | 'trash'
  | 'pencil'
  | 'upload'
  | 'refresh'
  | 'wifi-off'
  | 'clock'
  | 'lock'
  | 'logout'
  | 'spinner'
  | 'arrow-up'
  | 'arrow-down'
  | 'inbox'
  | 'folder'
  | 'command'
  | 'key'
  | 'dollar'
  | 'trending-up'
  | 'truck'
  | 'users'
  | 'sun'
  | 'calendar'
  | 'package'
  | 'sparkles'
  | 'history'
  | 'building'
  | 'hexagon'
  | 'menu'
  | 'sliders'
  | 'message-circle'
  | 'camera';

/** Inner SVG markup per icon (stroke paths; currentColor; 24×24 viewBox). */
export const ICON_PATHS: Record<IconName, string> = {
  check: '<path d="M20 6 9 17l-5-5"/>',
  'check-circle': '<circle cx="12" cy="12" r="9"/><path d="m9 12 2 2 4-4"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  'x-circle': '<circle cx="12" cy="12" r="9"/><path d="m15 9-6 6M9 9l6 6"/>',
  'alert-triangle':
    '<path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><path d="M12 9v4M12 17h.01"/>',
  'alert-circle': '<circle cx="12" cy="12" r="9"/><path d="M12 8v4M12 16h.01"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>',
  eye: '<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>',
  'eye-off':
    '<path d="M9.88 5.09A9.8 9.8 0 0 1 12 5c6.5 0 10 7 10 7a13 13 0 0 1-2.16 2.92M6.06 7.06A13 13 0 0 0 2 12s3.5 7 10 7a9.8 9.8 0 0 0 4.12-.91"/><path d="M3 3l18 18M9.9 9.9a3 3 0 0 0 4.2 4.2"/>',
  'chevron-down': '<path d="m6 9 6 6 6-6"/>',
  'chevron-right': '<path d="m9 6 6 6-6 6"/>',
  'chevron-left': '<path d="m15 6-6 6 6 6"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
  bell: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21v-1a6 6 0 0 1 6-6h4a6 6 0 0 1 6 6v1"/>',
  shield: '<path d="M12 2 4 6v6c0 5 3.5 8 8 10 4.5-2 8-5 8-10V6z"/>',
  'shield-check':
    '<path d="M12 2 4 6v6c0 5 3.5 8 8 10 4.5-2 8-5 8-10V6z"/><path d="m9 12 2 2 4-4"/>',
  'hard-hat':
    '<path d="M2 18a10 10 0 0 1 20 0z"/><path d="M10 10V5a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v5"/><path d="M4 18v-3M20 18v-3"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  trash:
    '<path d="M3 6h18M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
  pencil: '<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z"/>',
  upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M17 8l-5-5-5 5M12 3v12"/>',
  refresh: '<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
  'wifi-off':
    '<path d="M2 2l20 20M8.5 16.5a5 5 0 0 1 7 0M5 12.86a10 10 0 0 1 5.17-2.7M19 12.86a10 10 0 0 0-4-2.55M12 20h.01"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5M21 12H9"/>',
  spinner: '<path d="M12 3a9 9 0 1 0 9 9" stroke-linecap="round"/>',
  'arrow-up': '<path d="M12 19V5M5 12l7-7 7 7"/>',
  'arrow-down': '<path d="M12 5v14M5 12l7 7 7-7"/>',
  inbox:
    '<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>',
  folder:
    '<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13c0 1.1.9 2 2 2z"/>',
  command:
    '<path d="M15 6a3 3 0 1 1 3 3h-3zM9 6a3 3 0 1 0-3 3h3zM15 18a3 3 0 1 0 3-3h-3zM9 18a3 3 0 1 1-3-3h3zM9 9h6v6H9z"/>',
  key: '<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6M15.5 7.5l3 3L22 7l-3-3"/>',
  dollar: '<path d="M12 1v22M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>',
  'trending-up': '<path d="M22 7l-8.5 8.5-5-5L2 17"/><path d="M16 7h6v6"/>',
  truck:
    '<path d="M1 3h15v13H1zM16 8h4l3 3v5h-7z"/><circle cx="5.5" cy="18.5" r="2.5"/><circle cx="18.5" cy="18.5" r="2.5"/>',
  users:
    '<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/>',
  sun: '<circle cx="12" cy="12" r="5"/><path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/>',
  calendar: '<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>',
  package:
    '<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><path d="M3.27 6.96 12 12l8.73-5.04M12 22V12"/>',
  sparkles: '<path d="M12 3l1.9 5.8L20 10l-6.1 1.2L12 17l-1.9-5.8L4 10l6.1-1.2z"/>',
  history: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/>',
  building:
    '<rect x="4" y="2" width="16" height="20" rx="1"/><path d="M9 22v-4h6v4M8 6h.01M12 6h.01M16 6h.01M8 10h.01M12 10h.01M16 10h.01M8 14h.01M12 14h.01M16 14h.01"/>',
  hexagon: '<path d="M12 2l8.66 5v10L12 22l-8.66-5V7z"/>',
  menu: '<path d="M3 12h18M3 6h18M3 18h18"/>',
  sliders: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
  'message-circle': '<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22z"/>',
  camera:
    '<path d="M3 8a2 2 0 0 1 2-2h2l1.5-2h7L17 6h2a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><circle cx="12" cy="12.5" r="3.5"/>',
};
