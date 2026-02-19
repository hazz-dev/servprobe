'use strict';

const REFRESH_MS = 30_000;

let services = [];
let selectedService = null;
let refreshTimer = null;

// --- DOM ---
const grid = document.getElementById('service-grid');
const detailView = document.getElementById('detail-view');
const detailTitle = document.getElementById('detail-title');
const detailInfo = document.getElementById('detail-info');
const detailTable = document.getElementById('detail-table');
const detailChart = document.getElementById('chart');
const refreshInfo = document.getElementById('refresh-info');
const closeBtn = document.getElementById('close-detail');

closeBtn.addEventListener('click', () => {
  selectedService = null;
  detailView.classList.remove('visible');
});

// --- Fetch helpers ---
async function apiFetch(path) {
  const resp = await fetch(path);
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
  const body = await resp.json();
  if (body.error) throw new Error(body.error);
  return body.data;
}

// --- Render ---
function statusClass(status) {
  return status === 'up' ? 'up' : status === 'down' ? 'down' : 'unknown';
}

function fmtMs(ms) {
  if (ms == null || ms === 0) return '—';
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(2)}s`;
}

function fmtTime(ts) {
  if (!ts) return '—';
  return new Date(ts).toLocaleTimeString();
}

function renderCard(svc) {
  const cls = statusClass(svc.status);
  const card = document.createElement('div');
  card.className = 'card';
  card.dataset.name = svc.name;
  card.innerHTML = `
    <div class="card-header">
      <div class="status-dot ${cls}"></div>
      <span class="card-name">${svc.name}</span>
      <span class="type-badge">${svc.type}</span>
    </div>
    <div class="card-meta">
      <span>Status: <span class="val ${cls}">${svc.status || 'unknown'}</span></span>
      <span>Uptime: <span class="val">${svc.uptime_percent != null ? svc.uptime_percent.toFixed(1) + '%' : '—'}</span></span>
      <span>Response: <span class="val">${fmtMs(svc.response_ms)}</span></span>
      <span>Last check: <span class="val">${fmtTime(svc.last_checked)}</span></span>
    </div>`;
  card.addEventListener('click', () => showDetail(svc.name));
  return card;
}

function renderGrid() {
  grid.innerHTML = '';
  if (services.length === 0) {
    grid.innerHTML = '<p class="loading">No services configured.</p>';
    return;
  }
  services.forEach(svc => grid.appendChild(renderCard(svc)));
}

function drawChart(checks) {
  const canvas = detailChart;
  const ctx = canvas.getContext('2d');
  canvas.width = canvas.offsetWidth || 600;
  canvas.height = 120;

  ctx.clearRect(0, 0, canvas.width, canvas.height);

  if (!checks || checks.length === 0) return;

  const maxMs = Math.max(...checks.map(c => c.response_ms || 0), 1);
  const w = canvas.width;
  const h = canvas.height - 10;
  const step = w / (checks.length - 1 || 1);

  // Draw line
  ctx.beginPath();
  checks.forEach((c, i) => {
    const x = i * step;
    const y = h - (c.response_ms / maxMs) * h;
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.strokeStyle = '#00d4aa66';
  ctx.lineWidth = 2;
  ctx.stroke();

  // Draw dots
  checks.forEach((c, i) => {
    const x = i * step;
    const y = h - (c.response_ms / maxMs) * h;
    ctx.beginPath();
    ctx.arc(x, y, 3, 0, Math.PI * 2);
    ctx.fillStyle = c.status === 'up' ? '#00d4aa' : '#ff6b6b';
    ctx.fill();
  });
}

async function showDetail(name) {
  selectedService = name;
  detailView.classList.add('visible');
  detailTitle.textContent = name;
  detailInfo.innerHTML = '<span style="color:var(--text-muted)">Loading…</span>';
  detailTable.querySelector('tbody').innerHTML = '';

  try {
    const [svc, histResp] = await Promise.all([
      apiFetch(`/api/services/${encodeURIComponent(name)}`),
      apiFetch(`/api/services/${encodeURIComponent(name)}/history?limit=50`),
    ]);

    detailInfo.innerHTML = `
      <div>Type<span>${svc.type}</span></div>
      <div>Target<span>${svc.target}</span></div>
      <div>Interval<span>${svc.interval}</span></div>
      <div>Status<span class="${statusClass(svc.status)}">${svc.status || 'unknown'}</span></div>
      <div>Uptime (100 checks)<span>${svc.uptime_percent != null ? svc.uptime_percent.toFixed(1) + '%' : '—'}</span></div>`;

    const checks = (histResp.checks || []).slice().reverse();
    drawChart(checks);

    const tbody = detailTable.querySelector('tbody');
    tbody.innerHTML = '';
    (histResp.checks || []).forEach(c => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td class="${c.status}">${c.status}</td>
        <td>${fmtMs(c.response_ms)}</td>
        <td>${c.error || ''}</td>
        <td>${new Date(c.checked_at).toLocaleString()}</td>`;
      tbody.appendChild(tr);
    });
  } catch (e) {
    detailInfo.innerHTML = `<span style="color:var(--red)">${e.message}</span>`;
  }
}

// --- Main refresh loop ---
async function refresh() {
  try {
    services = await apiFetch('/api/services');
    renderGrid();
    refreshInfo.textContent = `Last updated: ${new Date().toLocaleTimeString()}`;
    if (selectedService) {
      showDetail(selectedService);
    }
  } catch (e) {
    refreshInfo.textContent = `Error: ${e.message}`;
  }
}

refresh();
refreshTimer = setInterval(refresh, REFRESH_MS);
