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

  function applyHost(ctx) {
    if (!ctx) return;
    // Follow the host's theme unless the person picked one in the page.
    let choice = null;
    try { choice = localStorage.getItem('cav-review-theme'); } catch {}
    if ((!choice || choice === 'system') && (ctx.theme === 'light' || ctx.theme === 'dark')) document.documentElement.dataset.theme = ctx.theme;
    // host-context-changed carries only the fields that changed: keep the height without one.
    const d = ctx.containerDimensions;
    if (!d) return;
    const h = d.height || Math.min(d.maxHeight || 640, 640);
    document.documentElement.style.setProperty('--cav-app-h', h + 'px');
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
      text: `I sent ${n} review note${n > 1 ? 's' : ''} on this video: ${video}\nRead the notes with \`cav review wait\` and that path as its argument, fix them, then resolve each one.` };
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
      if (!v.startsWith('/')) { delete this.dataset.cavSrc; desc.set.call(this, v); return; }
      this.dataset.cavSrc = v;
      dataURL(v).then(u => { if (this.dataset.cavSrc === v) desc.set.call(this, u); })
        .catch(e => showError(this, e.message));
    },
  });
  // A media request that fails (for example a preview too large for the chat) is shown in
  // the page: on the stage for the render itself, as a toast for the rest.
  function showError(el, msg) {
    const note = document.getElementById('stageNote'), toast = document.getElementById('toast');
    if (el.id === 'video' && note) {
      note.textContent = 'Could not load the render: ' + msg;
      note.hidden = false; note.classList.add('bad');
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
    document.querySelectorAll('img[src], video[src]').forEach(adopt);
    new ResizeObserver(report).observe(document.documentElement);
  });

  request('ui/initialize', { appInfo: { name: 'cav review', version: '1' }, appCapabilities: {}, protocolVersion: PROTOCOL })
    .then(r => { applyHost(r && r.hostContext); notify('ui/notifications/initialized', {}); })
    .catch(e => console.warn('cav: ui/initialize failed', e.message));
})();
