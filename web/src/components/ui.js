// ─────────────────────────────────────────────────────────────────────────────
// ui.js — Backwards-compatible DOM helpers for setup.js (non-JSX pages)
// JSX components import directly from './ui.jsx'
// ─────────────────────────────────────────────────────────────────────────────

// Re-export all JSX components for consumers that can use them
export * from './ui.jsx';

const ICONS = {
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  chevron: '<path d="m9 18 6-6-6-6"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  refresh: '<path d="M20 6v5h-5M4 18v-5h5"/><path d="M18.5 9A7 7 0 0 0 6 6.5L4 11M5.5 15A7 7 0 0 0 18 17.5l2-4.5"/>',
  download: '<path d="M12 3v12M7 10l5 5 5-5M4 21h16"/>',
  shield: '<path d="M12 3 4.5 6v5.3c0 4.6 3.1 8.8 7.5 9.7 4.4-.9 7.5-5.1 7.5-9.7V6z"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  warning: '<path d="M10.3 4.3 2.7 18a2 2 0 0 0 1.8 3h15a2 2 0 0 0 1.8-3L13.7 4.3a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
  play: '<polygon points="5 3 19 12 5 21 5 3"/>'
};

// Real DOM element builder — used by setup.js and any legacy non-JSX file
export function h(tag, attrs = {}, children = []) {
  const element = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (key === 'class') element.className = value;
    else if (key === 'text') element.textContent = value;
    else if (key.startsWith('on') && typeof value === 'function') element.addEventListener(key.slice(2).toLowerCase(), value);
    else if (key !== 'style' && value !== false && value !== null && value !== undefined) element.setAttribute(key, String(value));
  }
  for (const child of Array.isArray(children) ? children : [children]) {
    if (child === null || child === undefined || child === '') continue;
    if (child.nodeType) element.append(child);
    else element.append(document.createTextNode(String(child)));
  }
  return element;
}

// Real DOM icon builder — used by setup.js
export function icon(name, size = 16, label = '') {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('width', size);
  svg.setAttribute('height', size);
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '2');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('class', 'icon');
  if (label) { svg.setAttribute('role', 'img'); svg.setAttribute('aria-label', label); }
  else svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = ICONS[name] || ICONS.info;
  return svg;
}

// DOM-based card for legacy use
export function card(title, body, opts = {}) {
  const el = h('section', { class: `card ${opts.class || ''}`, 'aria-label': title });
  const header = h('div', { class: 'card-header' });
  const heading = h('div', { class: 'card-heading' });
  heading.append(h('h2', { text: title }));
  if (opts.subtitle) heading.append(h('p', { class: 'card-subtitle', text: opts.subtitle }));
  header.append(heading);
  if (opts.action) header.append(opts.action);
  const bodyEl = h('div', { class: `card-body ${opts.noPadding ? 'no-padding' : ''}` });
  if (Array.isArray(body)) body.filter(Boolean).forEach((c) => bodyEl.append(c));
  else if (body) bodyEl.append(body);
  el.append(header, bodyEl);
  return el;
}

// DOM-based pageHeader for legacy use
export function pageHeader({ title, description = '', actions = [] }) {
  const el = h('header', { class: 'page-header' });
  const main = h('div', { class: 'page-header-main' });
  main.append(h('h1', { text: title }));
  if (description) main.append(h('p', { class: 'page-description', text: description }));
  el.append(main);
  if (actions.length) {
    const act = h('div', { class: 'page-header-actions' });
    actions.forEach((a) => a && act.append(a));
    el.append(act);
  }
  return el;
}

// DOM-based badge for legacy use
export function badge(label, tone = 'neutral') {
  const el = h('span', { class: `badge ${tone}` });
  el.append(h('span', { class: 'badge-dot', 'aria-hidden': 'true' }));
  el.append(h('span', { text: label }));
  return el;
}

// DOM-based button for legacy use
export function button(label, onClick, tone = 'primary', opts = {}) {
  const children = [];
  if (opts.icon && ICONS[opts.icon]) children.push(icon(opts.icon, 14));
  children.push(h('span', { text: label }));
  return h('button', {
    class: `btn ${tone} ${opts.class || ''}`,
    type: opts.type || 'button',
    onclick: onClick,
    disabled: opts.disabled || false,
    title: opts.title || null
  }, children);
}

// DOM-based iconButton for legacy use
export function iconButton(name, label, onClick, tone = '') {
  return h('button', { class: `icon-btn ${tone}`, type: 'button', onclick: onClick, 'aria-label': label, title: label }, icon(name, 16));
}

// Real mountAsync for setup.js — mounts async data into a real DOM root
export function mountAsync(container, task, render) {
  container.replaceChildren(domLoading());
  task().then((value) => {
    const result = render(value);
    if (result !== null && result !== undefined) {
      if (result.nodeType) {
        container.replaceChildren(result);
      } else {
        // Preact VNode returned — render into container via preact render
        import('preact').then(({ render: preactRender }) => {
          preactRender(result, container);
        });
      }
    }
  }).catch((error) => {
    container.replaceChildren(domErrorBox(error, () => mountAsync(container, task, render)));
  });
}

function domLoading(label = 'Loading') {
  return h('div', { class: 'loading', role: 'status', 'aria-live': 'polite' }, [h('span', { text: label })]);
}

function domErrorBox(error, retry) {
  const el = h('div', { class: 'card alert-box-danger', role: 'alert' });
  const header = h('div', { class: 'card-header' });
  header.append(h('h2', { class: 'text-danger', text: 'System Notice' }));
  const body = h('div', { class: 'card-body' });
  body.append(h('p', { text: error?.message || String(error) }));
  if (retry) {
    const retryWrap = h('div', { class: 'mt-md' });
    retryWrap.append(button('Retry', retry, 'secondary compact'));
    body.append(retryWrap);
  }
  el.append(header, body);
  return el;
}

export function statusIndicator(label, tone = 'neutral') {
  const el = h('span', { class: 'status-indicator' });
  el.append(h('span', { class: `status-dot ${tone}` }));
  el.append(h('span', { text: label }));
  return el;
}

export function statusDot(label, tone = 'neutral') { return statusIndicator(label, tone); }

export function dataTable(headers, rows) {
  const wrap = h('div', { class: 'data-table-wrap' });
  const table = h('table', { class: 'data-table' });
  const thead = h('thead');
  const headerRow = h('tr');
  headers.forEach((hdr) => {
    let cls = '';
    if (hdr.width === '120px') cls = 'cell-actions-w120';
    else if (hdr.width === '160px') cls = 'cell-actions-w160';
    else if (hdr.width === '200px') cls = 'cell-actions-w200';
    headerRow.append(h('th', { class: cls, text: hdr.label || hdr }));
  });
  thead.append(headerRow);
  const tbody = h('tbody');
  if (rows.length > 0) rows.forEach((row) => tbody.append(row));
  else {
    const empty = h('tr');
    empty.append(h('td', { colspan: headers.length, class: 'empty-cell' }, [h('span', { text: 'No records found' })]));
    tbody.append(empty);
  }
  table.append(thead, tbody);
  wrap.append(table);
  return wrap;
}

export function emptyState(title, text, action = null) {
  const el = h('div', { class: 'empty-state' });
  el.append(h('strong', { text: title }));
  if (text) el.append(h('p', { text }));
  if (action) { const wrap = h('div', { class: 'mt-sm' }); wrap.append(action); el.append(wrap); }
  return el;
}

export function segmentedNav(tabs, activeId, onSelect) {
  const el = h('div', { class: 'segmented-nav', role: 'tablist' });
  tabs.forEach((tab) => {
    const active = tab.id === activeId;
    const btn = h('button', {
      class: `segmented-tab ${active ? 'active' : ''}`,
      type: 'button',
      role: 'tab',
      'aria-selected': active ? 'true' : 'false',
      onclick: () => onSelect(tab.id)
    });
    btn.append(h('span', { text: tab.label }));
    if (tab.count !== undefined) btn.append(h('span', { class: 'segmented-tab-count', text: String(tab.count) }));
    el.append(btn);
  });
  return el;
}

export function metricCard(label, value, footerText = '', statusTone = 'neutral') {
  const el = h('article', { class: 'metric-card' });
  el.append(h('div', { class: 'metric-card-label', text: label }));
  el.append(h('div', { class: 'metric-card-value', text: value }));
  if (footerText) {
    const footer = h('div', { class: 'metric-card-footer' });
    if (statusTone !== 'neutral') footer.append(h('span', { class: `status-dot ${statusTone}` }));
    footer.append(h('span', { text: footerText }));
    el.append(footer);
  }
  return el;
}

export function stat(label, value, hint = '', tone = '') { return metricCard(label, value, hint, tone); }

export function loading(label = 'Loading') { return domLoading(label); }

export function errorBox(error, retry) { return domErrorBox(error, retry); }

export function skeleton(variant = 'text') {
  return h('span', { class: `skeleton skeleton-${variant}`, 'aria-hidden': 'true' });
}
