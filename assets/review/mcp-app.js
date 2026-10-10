// The MCP App shim for the review page. `cav mcp` puts this script in the page's <head>,
// before the page's own script. The chat host draws the page in a sandboxed frame with no
// network, so this script answers the page's requests through the host instead:
//   - fetch('/api/...') becomes a call to the app-only review_request tool;
//   - <video> and <img> sources under / are fetched the same way and set as data: URLs.
// It speaks MCP Apps (protocol 2026-01-26) as JSON-RPC over postMessage, without the SDK.
(() => {
  const PROTOCOL = '2026-01-26';
  const TOOL = 'review_request';
  const STATE_TTL = 10000; // the page polls /api/state every 3 s; each poll is a tool call
  document.documentElement.classList.add('cav-mcp');

  // ---------- JSON-RPC over postMessage ----------
  let nextId = 1;
  const pending = new Map();
  const handlers = {};
  const post = msg => window.parent.postMessage(Object.assign({ jsonrpc: '2.0' }, msg), '*');
  const request = (method, params) => new Promise((resolve, reject) => {
    const id = nextId++;
    pending.set(id, { resolve, reject });
    post({ id, method, params });
  });
  const notify = (method, params) => post({ method, params });
  window.addEventListener('message', e => {
    if (e.source !== window.parent) return;
    const m = e.data;
    if (!m || m.jsonrpc !== '2.0') return;
    if (m.id != null && pending.has(m.id) && !m.method) {
      const p = pending.get(m.id);
      pending.delete(m.id);
      if (m.error) p.reject(new Error(m.error.message || 'request failed'));
      else p.resolve(m.result);
      return;
    }
    if (m.method && handlers[m.method]) handlers[m.method](m.params || {});
    else if (m.method && m.id != null) post({ id: m.id, result: {} }); // e.g. ui/resource-teardown
  });

  // ---------- the video under review, from the show_review result ----------
  let video = null, onVideo;
  const ready = new Promise(r => { onVideo = r; });
  handlers['ui/notifications/tool-result'] = p => {
    const v = p.structuredContent && p.structuredContent.video;
    if (v && !video) { video = v; onVideo(v); }
  };
  handlers['ui/notifications/host-context-changed'] = applyHost;

  // In the chat, the page's "follow the system" follows the host. A theme the person picked
  // that differs from the host's gets the page's own opaque paper (cav-own-theme), since the
  // conversation behind a transparent page would be the wrong shade for its ink.
  let hostTheme = null;

  // The page's tokens drawn from the host's style variables: [page token, value, host
  // tokens it needs]. Set inline on <html>, they outrank the page's :root rules.
  const hostVars = new Set();
  const v = n => `var(${n})`;
  const mix = (pct, a, b) => `color-mix(in srgb, ${v(a)} ${pct}%, ${b ? v(b) : 'transparent'})`;
  const HOST_TOKENS = [
    ['--paper', v('--color-background-primary'), '--color-background-primary'],
    ['--paper-2', v('--color-background-secondary'), '--color-background-secondary'],
    ['--paper-3', mix(10, '--color-text-primary', '--color-background-primary'), '--color-text-primary', '--color-background-primary'],
    ['--ink', v('--color-text-primary'), '--color-text-primary'],
    ['--ink-2', v('--color-text-secondary'), '--color-text-secondary'],
    ['--graphite', v('--color-text-tertiary'), '--color-text-tertiary'],
    ['--rule', v('--color-border-tertiary'), '--color-border-tertiary'],
    ['--rule-2', v('--color-border-secondary'), '--color-border-secondary'],
    ['--rule-3', v('--color-border-primary'), '--color-border-primary'],
    ['--sel', mix(4.5, '--color-text-primary'), '--color-text-primary'],
    ['--hover', mix(6, '--color-text-primary'), '--color-text-primary'],
    ['--ok', v('--color-text-success'), '--color-text-success'],
    ['--warn', v('--color-text-warning'), '--color-text-warning'],
    ['--bad', v('--color-text-danger'), '--color-text-danger'],
    ['--on-bad', v('--color-text-inverse'), '--color-text-inverse'],
    ['--tip', v('--color-background-inverse'), '--color-background-inverse'],
    ['--on-tip', v('--color-text-inverse'), '--color-text-inverse'],
    ['--sans', v('--font-sans'), '--font-sans'],
  ];
  // Hosts may send any subset: a page token follows the host only when every host token it
  // uses was sent, and keeps the page's own value otherwise. A theme picked in the page that
  // differs from the host's keeps the page's own colours, since host values need not adapt
  // to the page's color-scheme; the host font stays.
  function applyTokens() {
    const root = document.documentElement, own = root.classList.contains('cav-own-theme');
    for (const [token, value, ...needs] of HOST_TOKENS) {
      if (needs.every(n => hostVars.has(n)) && (!own || token === '--sans')) root.style.setProperty(token, value);
      else root.style.removeProperty(token);
    }
  }
  function syncTheme() {
    const root = document.documentElement;
    if (!root.dataset.theme && hostTheme) { root.dataset.theme = hostTheme; return; } // the observer runs again
    root.classList.toggle('cav-own-theme', !!hostTheme && root.dataset.theme !== hostTheme);
    applyTokens();
  }
  new MutationObserver(syncTheme).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });

  function applyHost(ctx) {
    if (!ctx) return;
    // Follow the host's theme unless the person picked one in the page.
    let choice = null;
    try { choice = localStorage.getItem('cav-review-theme'); } catch {}
    if (ctx.theme === 'light' || ctx.theme === 'dark') hostTheme = ctx.theme;
    if ((!choice || choice === 'system') && hostTheme) document.documentElement.dataset.theme = hostTheme;
    syncTheme();
    // The host's style variables and fonts.
    const st = ctx.styles;
    if (st && st.variables) {
      const root = document.documentElement.style;
      for (const [k, v] of Object.entries(st.variables)) if (k.startsWith('--') && v != null) { root.setProperty(k, String(v)); hostVars.add(k); }
      applyTokens();
    }
    if (st && st.css && st.css.fonts && !document.getElementById('cav-host-fonts')) {
      const el = document.createElement('style');
      el.id = 'cav-host-fonts';
      el.textContent = st.css.fonts;
      document.head.appendChild(el);
    }
    if (Array.isArray(ctx.availableDisplayModes)) hostModes = ctx.availableDisplayModes;
    if (ctx.displayMode) setMode(ctx.displayMode);
    else showFullBtn();
    // The composer can sit over the bottom of a full screen app: keep the page clear of it.
    const s = ctx.safeAreaInsets;
    if (s) for (const k of ['top', 'right', 'bottom', 'left']) document.documentElement.style.setProperty('--cav-safe-' + k, (s[k] || 0) + 'px');
    // host-context-changed carries only the fields that changed: keep the height without one.
    // Full screen fills the window by CSS: keep the inline height for the way back, since the
    // host may leave full screen without sending the dimensions again.
    const d = ctx.containerDimensions;
    if (!d || mode === 'fullscreen') return;
    const h = d.height || Math.min(d.maxHeight || 640, 640);
    document.documentElement.style.setProperty('--cav-app-h', h + 'px');
  }

  // ---------- full screen: a button in the title block, when the host offers it ----------
  // Inline, the host caps the frame at about 600 pixels, which leaves the stage small. Full
  // screen gives the stage and the sheet the whole window; the host draws its own close button.
  let hostModes = [], mode = 'inline', fullBtn = null;
  const ICON_EXPAND = 'M2.5 6V2.5H6M10 2.5h3.5V6M13.5 10v3.5H10M6 13.5H2.5V10';
  const ICON_SHRINK = 'M6 2.5V6H2.5M13.5 6H10V2.5M10 13.5V10h3.5M2.5 10H6v3.5';
  function setMode(m) {
    if (m !== 'inline' && m !== 'fullscreen' && m !== 'pip') return;
    mode = m;
    document.documentElement.classList.toggle('cav-full', m === 'fullscreen');
    showFullBtn();
  }
  function showFullBtn() {
    if (!fullBtn) return;
    const full = mode === 'fullscreen';
    fullBtn.hidden = !hostModes.includes('fullscreen');
    const label = full ? 'Exit full screen' : 'Full screen';
    fullBtn.setAttribute('aria-label', label);
    fullBtn.dataset.tip = label;
    fullBtn.querySelector('path').setAttribute('d', full ? ICON_SHRINK : ICON_EXPAND);
  }
  function addFullBtn() {
    const end = document.querySelector('.titleblock .end');
    if (!end) return;
    fullBtn = document.createElement('button');
    fullBtn.className = 'btn';
    fullBtn.id = 'fullscreen';
    fullBtn.dataset.tipPos = 'below';
    fullBtn.innerHTML = '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path/></svg>';
    fullBtn.addEventListener('click', () => {
      const want = mode === 'fullscreen' ? 'inline' : 'fullscreen';
      request('ui/request-display-mode', { mode: want })
        .then(r => setMode((r && r.mode) || want))
        .catch(e => console.warn('cav: ui/request-display-mode failed', e.message));
    });
    end.insertBefore(fullBtn, document.getElementById('help'));
    showFullBtn();
  }

  // ---------- requests through the review_request tool ----------
  async function call(method, path, body) {
    const v = await ready;
    const r = await request('tools/call', { name: TOOL, arguments: { video: v, method, path, body: body || '' } });
    const out = r && r.structuredContent;
    if (!out) throw new Error((r && r.content && r.content[0] && r.content[0].text) || 'no answer from cav');
    return out;
  }
  let stateCache = null;
  const realFetch = window.fetch.bind(window);
  window.fetch = async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url;
    if (!url.startsWith('/')) return realFetch(input, init);
    const method = (init.method || 'GET').toUpperCase();
    if (method === 'GET' && url === '/api/state' && stateCache && Date.now() - stateCache.at < STATE_TTL) return stateCache.res.clone();
    if (method !== 'GET') stateCache = null;
    const out = await call(method, url, typeof init.body === 'string' ? init.body : '');
    const res = new Response(out.body || '', { status: out.status, headers: { 'Content-Type': out.contentType || 'application/json' } });
    if (method === 'GET' && url === '/api/state' && res.ok) stateCache = { at: Date.now(), res: res.clone() };
    if (method === 'POST' && url === '/api/send' && res.ok) tellAgent(await res.clone().json().catch(() => null));
    return res;
  };

  // "Send to agent" in the chat: also say so in the conversation, so the agent picks the
  // notes up without waiting for the next prompt. Hosts that do not support ui/message
  // ignore it; the cav review hook still tells the agent.
  // The 2026-01-26 spec text sends one content block; the ext-apps SDK sends an array. Try
  // the spec's form first, and the array if the host refuses it.
  function tellAgent(send) {
    if (!send || !send.comments) return;
    const n = send.comments.length;
    const block = { type: 'text',
      // The path is named as data, not pasted into a command: the agent quotes it for its shell.
      text: `I sent ${n} review note${n > 1 ? 's' : ''} on this video: ${video}\nRead them with the review_notes tool, fix them, then resolve each one.` };
    request('ui/message', { role: 'user', content: block })
      .catch(() => request('ui/message', { role: 'user', content: [block] }))
      .catch(() => {});
  }

  // ---------- media: fetch once per path, hand the element a data: URL ----------
  const media = new Map();
  const dataURL = path => {
    if (!media.has(path)) media.set(path, call('GET', path, '').then(out => {
      if (out.status !== 200 || !out.dataUrl) {
        let msg = 'HTTP ' + out.status;
        try { msg = JSON.parse(out.body).error || msg; } catch {}
        throw new Error(msg);
      }
      return out.dataUrl;
    }).catch(e => { media.delete(path); throw e; }));
    return media.get(path);
  };
  // The page compares src with the path it set (for example to skip reloading the same
  // version), so the getter keeps returning that path.
  const desc = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, 'src');
  Object.defineProperty(HTMLMediaElement.prototype, 'src', {
    configurable: true,
    get() { return this.dataset.cavSrc ? new URL(this.dataset.cavSrc, 'http://cav').href : desc.get.call(this); },
    set(v) {
      v = String(v);
      if (!v.startsWith('/')) { delete this.dataset.cavSrc; delete this.dataset.cavLoaded; desc.set.call(this, v); return; }
      this.dataset.cavSrc = v;
      dataURL(v).then(u => { if (this.dataset.cavSrc === v) { this.dataset.cavLoaded = v; desc.set.call(this, u); } })
        .catch(e => { if (this.dataset.cavSrc === v) showError(this, e.message); }); // not if a newer path replaced it
    },
  });
  // A render the page has already replaced can still finish decoding while the new one is
  // fetched. Hide its loadeddata, so the page does not take notes on frames it no longer names.
  document.addEventListener('loadeddata', e => {
    const el = e.target;
    if (el instanceof HTMLMediaElement && el.dataset.cavSrc && el.dataset.cavLoaded !== el.dataset.cavSrc) e.stopImmediatePropagation();
  }, true);
  // A media request that fails (for example a preview too large for the chat) is shown in
  // the page: on the stage for the render itself, as a toast for the rest. A video also
  // drops the source it had, so the stage does not keep showing an earlier render that the
  // page no longer takes notes on; the page sees 'emptied' and stops adding notes.
  function showError(el, msg) {
    const note = document.getElementById('stageNote'), toast = document.getElementById('toast');
    if (el instanceof HTMLMediaElement && desc.get.call(el)) {
      el.pause();
      el.removeAttribute('src');
      el.load();
    }
    if (el.id === 'video' && note) {
      note.textContent = 'Could not load the render: ' + msg;
      note.hidden = false; note.classList.add('bad');
    } else if (el instanceof HTMLImageElement) {
      // A missing snapshot is not worth an alert: the thumbnail keeps its empty frame.
      el.removeAttribute('src');
    } else if (toast) {
      toast.textContent = 'Could not load: ' + msg;
      toast.classList.add('on');
    }
  }
  const adopt = el => {
    const s = el.getAttribute && el.getAttribute('src');
    if (!s || !s.startsWith('/')) return;
    if (el instanceof HTMLMediaElement) { el.removeAttribute('src'); el.src = s; return; }
    if (el instanceof HTMLImageElement) {
      el.removeAttribute('src');
      dataURL(s).then(u => { el.src = u; }).catch(e => showError(el, e.message));
    }
  };
  new MutationObserver(records => {
    for (const r of records) {
      if (r.type === 'attributes') adopt(r.target);
      for (const n of r.addedNodes || []) {
        if (n.nodeType !== 1) continue;
        adopt(n);
        n.querySelectorAll && n.querySelectorAll('img[src], video[src]').forEach(adopt);
      }
    }
  }).observe(document.documentElement, { subtree: true, childList: true, attributes: true, attributeFilter: ['src'] });

  // ---------- size: tell the host how tall the page is ----------
  let lastH = 0;
  const report = () => {
    const r = document.documentElement.getBoundingClientRect();
    const h = Math.ceil(r.height);
    if (h !== lastH) { lastH = h; notify('ui/notifications/size-changed', { width: Math.ceil(r.width), height: h }); }
  };
  addEventListener('DOMContentLoaded', () => {
    addFullBtn();
    document.querySelectorAll('img[src], video[src]').forEach(adopt);
    new ResizeObserver(report).observe(document.documentElement);
  });

  request('ui/initialize', { appInfo: { name: 'cav review', version: '1' }, appCapabilities: { availableDisplayModes: ['inline', 'fullscreen'] }, protocolVersion: PROTOCOL })
    .then(r => { applyHost(r && r.hostContext); notify('ui/notifications/initialized', {}); })
    .catch(e => console.warn('cav: ui/initialize failed', e.message));
})();
