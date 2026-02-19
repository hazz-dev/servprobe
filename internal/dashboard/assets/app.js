'use strict';

const REFRESH_MS = 30_000;

let services = [];
let selectedService = null;
let refreshTimer = null;

// --- DOM ---
const grid = document.getElementById('service-grid');
const overlay = document.getElementById('detail-overlay');
const detailTitle = document.getElementById('detail-title');
const detailDot = document.getElementById('detail-dot');
const detailType = document.getElementById('detail-type');
const detailInfo = document.getElementById('detail-info');
const detailTable = document.getElementById('detail-table');
const detailChart = document.getElementById('chart');
const refreshInfo = document.getElementById('refresh-info');
const closeBtn = document.getElementById('close-detail');
const countUp = document.getElementById('count-up');
const countDown = document.getElementById('count-down');

closeBtn.addEventListener('click', closeDetail);
overlay.addEventListener('click', (e) => {
  if (e.target === overlay) closeDetail();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeDetail();
});

function closeDetail() {
  selectedService = null;
  overlay.classList.remove('visible');
}

// --- Fetch helpers ---
async function apiFetch(path) {
  const resp = await fetch(path);
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const body = await resp.json();
  if (body.error) throw new Error(body.error);
  return body.data;
}

// --- Render helpers ---
function statusClass(status) {
  return status === 'up' ? 'up' : status === 'down' ? 'down' : 'unknown';
}

function fmtMs(ms) {
  if (ms == null || ms === 0) return '—';
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(2)}s`;
}

function fmtTime(ts) {
  if (!ts) return '—';
  const d = new Date(ts);
  return isNaN(d.getTime()) ? '—' : d.toLocaleTimeString();
}

function fmtDateTime(ts) {
  if (!ts) return '—';
  const d = new Date(ts);
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function uptimeClass(pct) {
  if (pct >= 99) return 'high';
  if (pct >= 90) return 'mid';
  return 'low';
}

function renderCard(svc) {
  const cls = statusClass(svc.status);
  const card = document.createElement('div');
  card.className = `card ${svc.status === 'down' ? 'status-down' : ''}`;
  card.dataset.name = svc.name;

  const uptimePct = svc.uptime_percent != null ? svc.uptime_percent : 0;

  card.innerHTML = `
    <div class="card-header">
      <div class="status-dot ${cls}"></div>
      <span class="card-name">${svc.name}</span>
      <span class="type-badge">${svc.type}</span>
    </div>
    <div class="card-meta">
      <div class="meta-item">
        <span class="meta-label">Status</span>
        <span class="meta-value ${cls}">${(svc.status || 'unknown').toUpperCase()}</span>
      </div>
      <div class="meta-item">
        <span class="meta-label">Response</span>
        <span class="meta-value">${fmtMs(svc.response_ms)}</span>
      </div>
      <div class="meta-item">
        <span class="meta-label">Uptime</span>
        <span class="meta-value">${uptimePct.toFixed(1)}%</span>
      </div>
      <div class="meta-item">
        <span class="meta-label">Last Check</span>
        <span class="meta-value">${fmtTime(svc.last_checked)}</span>
      </div>
    </div>
    <div class="uptime-bar">
      <div class="uptime-fill ${uptimeClass(uptimePct)}" style="width:${uptimePct}%"></div>
    </div>`;

  card.addEventListener('click', () => showDetail(svc.name));
  return card;
}

function renderGrid() {
  grid.innerHTML = '';
  if (services.length === 0) {
    grid.innerHTML = '<div class="loading-spinner"><p>No services configured.</p></div>';
    return;
  }
  services.forEach(svc => grid.appendChild(renderCard(svc)));

  // Update summary counts
  const up = services.filter(s => s.status === 'up').length;
  const down = services.filter(s => s.status === 'down').length;
  countUp.textContent = `${up} up`;
  if (down > 0) {
    countDown.textContent = `${down} down`;
    countDown.style.display = '';
  } else {
    countDown.style.display = 'none';
  }
}

function drawChart(checks) {
  const canvas = detailChart;
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.parentElement.getBoundingClientRect();
  canvas.width = rect.width * dpr;
  canvas.height = 140 * dpr;
  canvas.style.width = rect.width + 'px';
  canvas.style.height = '140px';
  ctx.scale(dpr, dpr);

  const w = rect.width;
  const h = 130;

  ctx.clearRect(0, 0, w, 150);

  if (!checks || checks.length < 2) {
    ctx.fillStyle = '#6b7a99';
    ctx.font = '13px Inter, sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('Not enough data for chart', w / 2, h / 2);
    return;
  }

  const maxMs = Math.max(...checks.map(c => c.response_ms || 0), 1);
  const padY = 10;
  const step = w / (checks.length - 1);

  // Draw gradient fill
  const gradient = ctx.createLinearGradient(0, 0, 0, h);
  gradient.addColorStop(0, 'rgba(59, 130, 246, 0.15)');
  gradient.addColorStop(1, 'rgba(59, 130, 246, 0)');

  ctx.beginPath();
  checks.forEach((c, i) => {
    const x = i * step;
    const y = padY + (h - padY * 2) - ((c.response_ms || 0) / maxMs) * (h - padY * 2);
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  // Close for fill
  ctx.lineTo((checks.length - 1) * step, h);
  ctx.lineTo(0, h);
  ctx.closePath();
  ctx.fillStyle = gradient;
  ctx.fill();

  // Draw line
  ctx.beginPath();
  checks.forEach((c, i) => {
    const x = i * step;
    const y = padY + (h - padY * 2) - ((c.response_ms || 0) / maxMs) * (h - padY * 2);
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.strokeStyle = '#3b82f6';
  ctx.lineWidth = 2;
  ctx.lineJoin = 'round';
  ctx.stroke();

  // Draw dots
  checks.forEach((c, i) => {
    const x = i * step;
    const y = padY + (h - padY * 2) - ((c.response_ms || 0) / maxMs) * (h - padY * 2);
    ctx.beginPath();
    ctx.arc(x, y, 3, 0, Math.PI * 2);
    ctx.fillStyle = c.status === 'up' ? '#22c55e' : '#ef4444';
    ctx.fill();
  });
}

async function showDetail(name) {
  selectedService = name;
  overlay.classList.add('visible');
  detailTitle.textContent = name;
  detailInfo.innerHTML = '<div class="stat-card" style="grid-column:1/-1"><div class="stat-value" style="color:var(--text-muted)">Loading…</div></div>';
  detailTable.querySelector('tbody').innerHTML = '';

  try {
    const [svc, histResp] = await Promise.all([
      apiFetch(`/api/services/${encodeURIComponent(name)}`),
      apiFetch(`/api/services/${encodeURIComponent(name)}/history?limit=50`),
    ]);

    const cls = statusClass(svc.status);
    detailDot.className = `detail-status-dot ${cls}`;
    detailType.textContent = svc.type;

    detailInfo.innerHTML = `
      <div class="stat-card">
        <div class="stat-label">Status</div>
        <div class="stat-value" style="color:var(--${cls === 'up' ? 'green' : cls === 'down' ? 'red' : 'text-muted'})">${(svc.status || 'unknown').toUpperCase()}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Target</div>
        <div class="stat-value" style="font-size:0.8rem;word-break:break-all">${svc.target}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Interval</div>
        <div class="stat-value">${svc.interval}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Uptime</div>
        <div class="stat-value">${svc.uptime_percent != null ? svc.uptime_percent.toFixed(1) + '%' : '—'}</div>
      </div>`;

    const checks = (histResp.checks || []).slice().reverse();
    drawChart(checks);

    const tbody = detailTable.querySelector('tbody');
    tbody.innerHTML = '';
    (histResp.checks || []).forEach(c => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td><span class="status-badge ${statusClass(c.status)}">${(c.status || '?').toUpperCase()}</span></td>
        <td>${fmtMs(c.response_ms)}</td>
        <td style="color:var(--text-muted)">${c.error || '—'}</td>
        <td>${fmtDateTime(c.checked_at)}</td>`;
      tbody.appendChild(tr);
    });
  } catch (e) {
    detailInfo.innerHTML = `<div class="stat-card" style="grid-column:1/-1"><div class="stat-value" style="color:var(--red)">${e.message}</div></div>`;
  }
}

// --- Main refresh loop ---
async function refresh() {
  try {
    services = await apiFetch('/api/services');
    renderGrid();
    refreshInfo.textContent = `Updated ${new Date().toLocaleTimeString()}`;
    if (selectedService) {
      showDetail(selectedService);
    }
  } catch (e) {
    refreshInfo.textContent = `Error: ${e.message}`;
  }
}

refresh();
refreshTimer = setInterval(refresh, REFRESH_MS);
