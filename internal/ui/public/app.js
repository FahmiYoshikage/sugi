/**
 * Sugi Observability Engine - Vanilla JS Client & Canvas Charting
 * Zero NPM dependencies, zero CDN, 100% offline-ready.
 */

// Application State
const MAX_HISTORY = 60; // 60 data points (1 minute window)
const state = {
  connected: false,
  history: {
    cpuTotal: [],
    cpuUser: [],
    cpuSys: [],
    memUsedMB: [],
    memAvailMB: [],
    diskReadKB: [],
    diskWriteKB: [],
    netRxKB: [],
    netTxKB: []
  },
  logs: []
};

// --- Custom Lightweight Canvas Chart Engine ---
class MiniChart {
  constructor(canvasId, options = {}) {
    this.canvas = document.getElementById(canvasId);
    if (!this.canvas) return;
    this.ctx = this.canvas.getContext('2d');
    this.options = Object.assign({
      unit: '%',
      fixedMax: null,
      series: [] // [{ key, color, fillColor }]
    }, options);
    this.setupDPI();
    window.addEventListener('resize', () => {
      this.setupDPI();
      this.render();
    });
  }

  setupDPI() {
    if (!this.canvas) return;
    const rect = this.canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    this.width = rect.width;
    this.height = rect.height;
    this.canvas.width = this.width * dpr;
    this.canvas.height = this.height * dpr;
    this.ctx.resetTransform?.();
    this.ctx.scale(dpr, dpr);
  }

  render() {
    if (!this.ctx || !this.width) return;
    const ctx = this.ctx;
    const w = this.width;
    const h = this.height;

    ctx.clearRect(0, 0, w, h);

    // Calculate dynamic max across active series
    let maxVal = this.options.fixedMax || 1;
    if (!this.options.fixedMax) {
      for (const s of this.options.series) {
        const data = state.history[s.key] || [];
        for (const v of data) {
          if (v > maxVal) maxVal = v;
        }
      }
      maxVal = Math.ceil(maxVal * 1.15) || 10;
    }

    // Draw grid lines
    ctx.strokeStyle = '#21262d';
    ctx.lineWidth = 1;
    const gridLines = 4;
    for (let i = 0; i <= gridLines; i++) {
      const y = h - (i * (h - 20) / gridLines) - 10;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(w, y);
      ctx.stroke();

      // Y-axis label
      const labelVal = ((i / gridLines) * maxVal).toFixed(maxVal < 5 ? 1 : 0);
      ctx.fillStyle = '#6e7681';
      ctx.font = '10px ui-monospace, monospace';
      ctx.textAlign = 'right';
      ctx.fillText(labelVal + this.options.unit, w - 4, y - 3);
    }

    // Draw Series Lines and Areas
    for (const s of this.options.series) {
      const data = state.history[s.key] || [];
      if (data.length < 2) continue;

      const step = w / (MAX_HISTORY - 1);
      const points = [];

      const startOffset = MAX_HISTORY - data.length;
      for (let i = 0; i < data.length; i++) {
        const x = (startOffset + i) * step;
        const normalized = Math.min(Math.max(data[i], 0), maxVal) / maxVal;
        const y = h - 10 - (normalized * (h - 20));
        points.push({ x, y });
      }

      // Draw Gradient Area
      if (s.fillColor) {
        const grad = ctx.createLinearGradient(0, 0, 0, h);
        grad.addColorStop(0, s.fillColor);
        grad.addColorStop(1, 'transparent');

        ctx.beginPath();
        ctx.moveTo(points[0].x, h - 10);
        for (const pt of points) ctx.lineTo(pt.x, pt.y);
        ctx.lineTo(points[points.length - 1].x, h - 10);
        ctx.closePath();
        ctx.fillStyle = grad;
        ctx.fill();
      }

      // Draw Stroke Line
      ctx.beginPath();
      ctx.moveTo(points[0].x, points[0].y);
      for (let i = 1; i < points.length; i++) {
        ctx.lineTo(points[i].x, points[i].y);
      }
      ctx.strokeStyle = s.color;
      ctx.lineWidth = 2;
      ctx.stroke();
    }
  }
}

// Instantiate charts
let cpuChart, memChart, diskChart, netChart;

function initCharts() {
  cpuChart = new MiniChart('cpu-chart', {
    unit: '%',
    fixedMax: 100,
    series: [
      { key: 'cpuTotal', color: '#3fb950', fillColor: 'rgba(63, 185, 80, 0.25)' },
      { key: 'cpuUser', color: '#58a6ff' },
      { key: 'cpuSys', color: '#bc8cff' }
    ]
  });

  memChart = new MiniChart('mem-chart', {
    unit: 'MB',
    series: [
      { key: 'memUsedMB', color: '#58a6ff', fillColor: 'rgba(88, 166, 255, 0.2)' }
    ]
  });

  diskChart = new MiniChart('disk-chart', {
    unit: 'KB/s',
    series: [
      { key: 'diskReadKB', color: '#3fb950' },
      { key: 'diskWriteKB', color: '#d29922', fillColor: 'rgba(210, 153, 34, 0.2)' }
    ]
  });

  netChart = new MiniChart('net-chart', {
    unit: 'KB/s',
    series: [
      { key: 'netRxKB', color: '#3fb950', fillColor: 'rgba(63, 185, 80, 0.2)' },
      { key: 'netTxKB', color: '#58a6ff' }
    ]
  });
}

function updateHistory(key, val) {
  const arr = state.history[key];
  arr.push(val);
  if (arr.length > MAX_HISTORY) arr.shift();
}

function renderAllCharts() {
  cpuChart?.render();
  memChart?.render();
  diskChart?.render();
  netChart?.render();
}

// --- SSE EventSource Connection ---
function setupSSE() {
  const statusBadge = document.getElementById('status-badge');
  const statusText = document.getElementById('status-text');

  const es = new EventSource('/api/v1/stream');

  es.onopen = () => {
    state.connected = true;
    if (statusBadge) statusBadge.style.display = 'inline-flex';
    if (statusText) statusText.innerText = 'Live SSE Connected';
  };

  es.addEventListener('metrics', (e) => {
    try {
      const snap = JSON.parse(e.data);
      onMetricSnapshot(snap);
    } catch (err) {
      console.error('SSE metrics parse error:', err);
    }
  });

  es.addEventListener('log', (e) => {
    try {
      const entry = JSON.parse(e.data);
      prependLiveLog(entry);
    } catch (err) {
      console.error('SSE log parse error:', err);
    }
  });

  es.onerror = () => {
    state.connected = false;
    if (statusText) statusText.innerText = 'Reconnecting...';
    if (statusBadge) {
      statusBadge.style.borderColor = 'rgba(210, 153, 34, 0.4)';
      statusBadge.style.color = '#d29922';
    }
  };
}

// Process Real-time Snapshot
function onMetricSnapshot(snap) {
  // 1. CPU
  const cpuVal = snap.cpu?.total_usage || 0;
  setText('val-cpu', cpuVal.toFixed(1) + '%');
  setText('sub-cpu', `User ${snap.cpu?.user_usage?.toFixed(1) || 0}% | Sys ${snap.cpu?.system_usage?.toFixed(1) || 0}%`);
  setBar('bar-cpu', cpuVal);
  updateHistory('cpuTotal', cpuVal);
  updateHistory('cpuUser', snap.cpu?.user_usage || 0);
  updateHistory('cpuSys', snap.cpu?.system_usage || 0);

  // Render Cores
  renderCores(snap.cpu?.cores || []);

  // 2. Memory
  const memUsedMB = Math.round((snap.memory?.used_bytes || 0) / (1024 * 1024));
  const memTotalMB = Math.round((snap.memory?.total_bytes || 0) / (1024 * 1024));
  const memPct = snap.memory?.used_percent || 0;
  setText('val-mem', `${memUsedMB} MB`);
  setText('sub-mem', `${memPct.toFixed(1)}% of ${(memTotalMB / 1024).toFixed(1)} GB`);
  setBar('bar-mem', memPct);
  updateHistory('memUsedMB', memUsedMB);

  // 3. Disk I/O
  const readKB = (snap.disk?.total_read_bytes_per_sec || 0) / 1024;
  const writeKB = (snap.disk?.total_write_bytes_per_sec || 0) / 1024;
  setText('val-disk', `${writeKB.toFixed(1)} KB/s`);
  setText('sub-disk', `Read: ${readKB.toFixed(1)} KB/s (${snap.disk?.total_read_iops || 0} IOPS)`);
  updateHistory('diskReadKB', readKB);
  updateHistory('diskWriteKB', writeKB);

  // 4. Network
  const rxKB = (snap.network?.total_rx_bytes_per_sec || 0) / 1024;
  const txKB = (snap.network?.total_tx_bytes_per_sec || 0) / 1024;
  setText('val-net', `${(rxKB + txKB).toFixed(1)} KB/s`);
  setText('sub-net', `Rx: ${rxKB.toFixed(1)} KB/s | Tx: ${txKB.toFixed(1)} KB/s`);
  updateHistory('netRxKB', rxKB);
  updateHistory('netTxKB', txKB);

  // Re-render Canvas Charts
  renderAllCharts();
}

function renderCores(cores) {
  const container = document.getElementById('core-grid');
  if (!container) return;

  if (container.children.length !== cores.length) {
    container.innerHTML = '';
    for (const core of cores) {
      const el = document.createElement('div');
      el.className = 'core-badge';
      el.id = 'badge-' + core.id;
      el.innerHTML = `
        <div class="core-name">${core.id}</div>
        <div class="core-pct">${core.total_usage.toFixed(0)}%</div>
      `;
      container.appendChild(el);
    }
  } else {
    for (const core of cores) {
      const el = document.getElementById('badge-' + core.id);
      if (el) {
        const pctEl = el.querySelector('.core-pct');
        if (pctEl) pctEl.innerText = core.total_usage.toFixed(0) + '%';
      }
    }
  }
}

// --- Log Explorer ---
async function fetchLogs() {
  const search = document.getElementById('log-search')?.value || '';
  const level = document.getElementById('log-level')?.value || '';

  const params = new URLSearchParams();
  if (search) params.set('search', search);
  if (level) params.set('level', level);
  params.set('limit', '50');

  try {
    const res = await fetch('/api/v1/logs?' + params.toString());
    if (!res.ok) return;
    const data = await res.json();
    renderLogs(data.logs || []);
  } catch (err) {
    console.error('Fetch logs error:', err);
  }
}

function renderLogs(logs) {
  const tbody = document.getElementById('log-tbody');
  if (!tbody) return;

  if (logs.length === 0) {
    tbody.innerHTML = `<tr><td colspan="5" style="text-align:center;color:#6e7681;padding:1rem;">No logs found. Ingest some logs via POST /api/v1/logs</td></tr>`;
    return;
  }

  tbody.innerHTML = logs.map(l => {
    const ts = new Date(l.timestamp).toLocaleTimeString();
    const pillClass = getPillClass(l.level);
    const attrStr = l.attributes ? JSON.stringify(l.attributes) : '-';
    return `
      <tr>
        <td style="color:#8b949e;">${escapeHTML(ts)}</td>
        <td><span class="pill ${pillClass}">${escapeHTML(l.level)}</span></td>
        <td style="color:#58a6ff;">${escapeHTML(l.service)}</td>
        <td>${escapeHTML(l.message)}</td>
        <td style="color:#8b949e;font-size:0.75rem;">${escapeHTML(attrStr)}</td>
      </tr>
    `;
  }).join('');
}

function getPillClass(lvl) {
  switch ((lvl || '').toUpperCase()) {
    case 'ERROR': return 'pill-error';
    case 'WARN': return 'pill-warn';
    case 'DEBUG': return 'pill-debug';
    default: return 'pill-info';
  }
}

// Prepend real-time live log from SSE
function prependLiveLog(l) {
  const tbody = document.getElementById('log-tbody');
  if (!tbody) return;

  const currentLevel = (document.getElementById('log-level')?.value || '').toUpperCase();
  const currentSearch = (document.getElementById('log-search')?.value || '').toLowerCase();

  if (currentLevel && (l.level || '').toUpperCase() !== currentLevel) {
    return;
  }
  if (currentSearch && !l.message?.toLowerCase().includes(currentSearch) && !l.service?.toLowerCase().includes(currentSearch)) {
    return;
  }

  // Clear 'No logs found' placeholder if present
  const firstRow = tbody.querySelector('tr td[colspan]');
  if (firstRow) {
    tbody.innerHTML = '';
  }

  const tr = document.createElement('tr');
  const ts = new Date(l.timestamp).toLocaleTimeString();
  const pillClass = getPillClass(l.level);
  const attrStr = l.attributes ? JSON.stringify(l.attributes) : '-';

  tr.innerHTML = `
    <td style="color:#8b949e;">${escapeHTML(ts)}</td>
    <td><span class="pill ${pillClass}">${escapeHTML(l.level)}</span></td>
    <td style="color:#58a6ff;">${escapeHTML(l.service)}</td>
    <td>${escapeHTML(l.message)}</td>
    <td style="color:#8b949e;font-size:0.75rem;">${escapeHTML(attrStr)}</td>
  `;

  tbody.insertBefore(tr, tbody.firstChild);

  // Keep max 100 rows in DOM
  while (tbody.children.length > 100) {
    tbody.removeChild(tbody.lastChild);
  }
}

// Simulate Log Burst for testing
async function simulateLogBurst() {
  const btn = document.getElementById('burst-btn');
  const origText = btn ? btn.innerHTML : '';
  if (btn) {
    btn.disabled = true;
    btn.innerText = 'Sending Burst...';
  }

  const sampleMessages = [
    { level: 'INFO', service: 'auth-api', msg: 'User session verified for user_id: 8192' },
    { level: 'WARN', service: 'auth-api', msg: 'Failed password attempt for invalid user admin from 198.51.100.22' },
    { level: 'INFO', service: 'payment-gw', msg: 'Stripe webhook received: payment_intent.succeeded' },
    { level: 'INFO', service: 'postgres-db', msg: 'WAL checkpoint completed: 42 pages synced in 14ms' },
    { level: 'WARN', service: 'redis-cache', msg: 'Evicted 48 keys due to maxmemory-policy volatile-lru' },
    { level: 'ERROR', service: 'worker-queue', msg: 'Job #9218 timed out after 30000ms: external webhook unreachable' },
    { level: 'INFO', service: 'nginx', msg: 'GET /api/v1/metrics 200 4.2ms - Mozilla/5.0' },
    { level: 'INFO', service: 'nginx', msg: 'POST /api/v1/checkout 201 12.8ms - curl/8.5.0' },
    { level: 'ERROR', service: 'auth-api', msg: 'JWT validation error: token expired' },
    { level: 'INFO', service: 'worker-queue', msg: 'Processed batch of 50 background notifications in 118ms' }
  ];

  const payload = sampleMessages.map(m => ({
    timestamp: new Date().toISOString(),
    level: m.level,
    service: m.service,
    message: m.msg,
    attributes: { environment: 'production', host: 'host-01' }
  }));

  try {
    await fetch('/api/v1/logs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
  } catch (err) {
    console.error('Failed to send log burst:', err);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerHTML = origText;
    }
  }
}

// Quick Log Form Submission
function setupLogForm() {
  const form = document.getElementById('log-form');
  if (!form) return;

  form.addEventListener('submit', async (e) => {
    e.preventDefault();

    // Anti-spam honeypot verification
    const hp = document.getElementById('hp_email');
    if (hp && hp.value) {
      console.warn('Spam submission detected by honeypot.');
      return;
    }

    const service = document.getElementById('form-service').value.trim() || 'webapp';
    const level = document.getElementById('form-level').value;
    const message = document.getElementById('form-message').value.trim();

    if (!message) {
      alert('Log message cannot be empty.');
      return;
    }

    const payload = {
      level,
      service,
      message,
      attributes: { source: 'web-dashboard' }
    };

    try {
      const res = await fetch('/api/v1/logs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (res.ok) {
        document.getElementById('form-message').value = '';
        setTimeout(fetchLogs, 250);
      }
    } catch (err) {
      alert('Failed to send log: ' + err.message);
    }
  });
}

// Helpers
function setText(id, text) {
  const el = document.getElementById(id);
  if (el) el.innerText = text;
}

function setBar(id, pct) {
  const el = document.getElementById(id);
  if (el) el.style.width = Math.min(Math.max(pct, 0), 100) + '%';
}

function escapeHTML(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// --- Initialization ---
document.addEventListener('DOMContentLoaded', () => {
  initCharts();
  setupSSE();
  setupLogForm();
  fetchLogs();

  // Search filter debounce
  let searchTimer;
  document.getElementById('log-search')?.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(fetchLogs, 300);
  });
  document.getElementById('log-level')?.addEventListener('change', fetchLogs);

  // Periodic log refresh
  setInterval(fetchLogs, 4000);
});
