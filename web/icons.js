// The product's icon set: the names a menu entry's State may hold in icon
// (ontology.yaml State.icon). Each is 24x24, drawn with currentColor
// strokes, so it follows the text colour. An unknown name draws a dot.
const PATHS = {
  home: 'M3 11l9-7 9 7M5 10v10h5v-6h4v6h5V10',
  graph: 'M6 6m-2.5 0a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0-5 0M18 7m-2.5 0a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0-5 0M12 18m-2.5 0a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0-5 0M8.3 7.2l7.2-.2M7.3 8.2l3.5 7.6M16.8 9.2l-3.6 6.6',
  emitter: 'M4 12h4l2-6 4 12 2-6h4',
  designer: 'M4 20l4-1 11-11-3-3L5 16l-1 4zM14 6l3 3',
  workflow: 'M4 5h6v5H4zM14 14h6v5h-6zM7 10v3a2 2 0 0 0 2 2h5',
  environment: 'M3 6h18v5H3zM3 13h18v5H3zM7 8.5h.01M7 15.5h.01',
  compliance: 'M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6l8-3zM8.5 12l2.5 2.5 4.5-5',
  policy: 'M12 3v18M5 7h14M5 7l-3 6a3 3 0 0 0 6 0L5 7zM19 7l-3 6a3 3 0 0 0 6 0l-3-6zM8 21h8',
  ledger: 'M5 4h11a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3V4zM9 9h6M9 13h6',
  identity: 'M12 8m-4 0a4 4 0 1 0 8 0a4 4 0 1 0-8 0M4 21c1-4 4.5-6 8-6s7 2 8 6',
  artifact: 'M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3zM4 7.5l8 4.5 8-4.5M12 12v9',
  stack: 'M12 3l9 5-9 5-9-5 9-5zM3 13l9 5 9-5M3 17.5l9 5 9-5',
  settings: 'M12 9a3 3 0 1 0 0 6a3 3 0 1 0 0-6zM12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1',
  search: 'M11 11m-6 0a6 6 0 1 0 12 0a6 6 0 1 0-12 0M20 20l-4.5-4.5',
  help: 'M12 12m-9 0a9 9 0 1 0 18 0a9 9 0 1 0-18 0M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6V14M12 17h.01',
  marker: 'M6 21V4h10l-2 4 2 4H6',
  'chevron-left': 'M15 5l-7 7 7 7',
  'chevron-right': 'M9 5l7 7-7 7',
  'chevron-down': 'M5 9l7 7 7-7',
};

// icon returns an <svg> for name, aria-hidden: the label beside it names it.
export function icon(name, size = 18) {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('width', size);
  svg.setAttribute('height', size);
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('class', 'pt-icon');
  const path = document.createElementNS(ns, 'path');
  path.setAttribute('d', PATHS[name] || 'M12 12m-2 0a2 2 0 1 0 4 0a2 2 0 1 0-4 0');
  svg.appendChild(path);
  return svg;
}
