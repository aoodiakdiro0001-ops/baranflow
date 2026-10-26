/* BaranFlow UI — Baran © 2026 */
const $ = (s) => document.querySelector(s);
const $$ = (s) => document.querySelectorAll(s);
let netHist = [], logTab = 'all', capsCache = null;

const fmtB = (n) => {
  n = Number(n) || 0;
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i ? n.toFixed(i > 1 ? 2 : 1) : Math.floor(n)) + ' ' + u[i];
};
const fmtS = (n) => { n = Number(n) || 0; return n <= 0 ? '0 B/s' : fmtB(n) + '/s'; };
const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));

async function api(p, o) {
  const r = await fetch(p, o);
  if (!r.ok) { const t = await r.text(); throw new Error(t.slice(0, 160)); }
  return r.json();
}

window.showToast = (msg) => {
  const t = $('#toast');
  t.textContent = msg;
  t.style.display = 'block';
  clearTimeout(t._h);
  t._h = setTimeout(() => (t.style.display = 'none'), 2600);
};

function switchTab(id) {
  $$('.panel').forEach(p => p.classList.toggle('show', p.id === 'p-' + id));
  $$('.nav-item').forEach(n => n.classList.toggle('active', n.dataset.t === id));
}
$$('#nav .nav-item').forEach(n => n.onclick = () => switchTab(n.dataset.t));

async function loadStats() {
  try {
    const s = await api('/api/stats');
    $('#sSpeed').textContent = fmtS(s.down_bps);
    $('#sActive').textContent = s.active;
    $('#sDone').textContent = s.done;
    $('#sFailed').textContent = s.failed;
    $('#sStorage').textContent = fmtB(s.storage_bytes || 0);
    $('#sDir').textContent = s.download_dir || '';
    $('#storageBar').style.width = Math.min(100, Math.max(4, (s.storage_bytes || 0) / (64 * 1024 * 1024 * 1024) * 100)) + '%';

    const types = Object.entries(s.types || {}).sort((a, b) => b[1] - a[1]);
    const total = types.reduce((x, [, v]) => x + v, 0);
    const colors = ['#6ea8ff', '#34d399', '#f59e0b', '#ef4444', '#a78bfa', '#22d3ee', '#f472b6', '#94a3b8'];
    let acc = 0; const segs = [];
    types.forEach(([k, v], i) => {
      const a0 = acc / total * 100; acc += v; const a1 = acc / total * 100;
      segs.push(`${colors[i % colors.length]} ${a0.toFixed(2)}% ${a1.toFixed(2)}%`);
    });
    $('#donut').style.background = total ? `conic-gradient(${segs.join(',')},#1e2530 0 100%)` : '#1e2530';
    $('#legend').innerHTML = total
      ? types.map(([k, v], i) => `<span class="lg"><i style="background:${colors[i % colors.length]}"></i>${esc(k)} (${v})</span>`).join('')
      : '<span class="muted">لا ملفات بعد — أضف مهمة</span>';

    netHist.push(s.down_bps || 0);
    if (netHist.length > 80) netHist.shift();
    drawNet();
  } catch (e) { console.warn(e); }
}

function drawNet() {
  const c = $('#net'); if (!c) return;
  const ctx = c.getContext('2d');
  const w = c.width = c.clientWidth, h = c.height = c.clientHeight;
  ctx.clearRect(0, 0, w, h);
  ctx.strokeStyle = '#1c222e'; ctx.lineWidth = 1;
  for (let i = 1; i < 4; i++) { ctx.beginPath(); ctx.moveTo(0, h * i / 4); ctx.lineTo(w, h * i / 4); ctx.stroke(); }
  if (!netHist.length) return;
  const max = Math.max(...netHist, 1024);
  ctx.beginPath();
  netHist.forEach((v, i) => {
    const x = 2 + i * (w - 4) / 79, y = h - 6 - (v / max) * (h - 20);
    i ? ctx.lineTo(x, y) : ctx.moveTo(x, y);
  });
  ctx.strokeStyle = '#6ea8ff'; ctx.lineWidth = 2; ctx.stroke();
  ctx.fillStyle = '#8b93a5'; ctx.font = '11px sans-serif'; ctx.textAlign = 'left';
  ctx.fillText(' الذروة: ' + fmtS(max), 8, 14);
}

async function loadTasks() {
  try {
    const d = await api('/api/tasks');
    const list = d.tasks || [];
    $('#tasksBody').innerHTML = list.map(t => {
      const pct = t.total > 0 ? Math.min(100, (t.done / t.total * 100)).toFixed(1) : (t.state === 'done' ? 100 : 0);
      const size = t.total ? `${fmtB(t.done)} / ${fmtB(t.total)}` : fmtB(t.done);
      const stMap = { queued: 'في الطابور', running: 'يعمل', done: 'مكتمل', failed: 'فشل', canceled: 'موقوف' };
      const pill = { queued: 'b', running: 'g', done: 'g', failed: 'r', canceled: 'y' }[t.state] || 'b';
      const act = (t.state === 'running' || t.state === 'queued')
        ? `<button class="btn sm warn" onclick="pauseTask('${t.id}')">إيقاف</button>` : '';
      return `<tr>
        <td><b>${esc(t.filename)}</b><br><small class="muted" dir="ltr">${esc(t.url).slice(0, 64)}</small></td>
        <td>${size}<div class="bar"><div style="width:${pct}%"></div></div></td>
        <td>${fmtS(t.speed_bps)}</td>
        <td><span class="pill ${pill}">${stMap[t.state] || t.state}</span>${t.err ? `<br><small class="muted">${esc(t.err)}</small>` : ''}</td>
        <td>${act}</td></tr>`;
    }).join('') || '<tr><td colspan="5" class="muted">لا مهام — الصق رابطًا بالأعلى واضغط تحليل</td></tr>';
  } catch (e) { console.warn(e); }
}

window.pauseTask = async (id) => {
  try { await api('/api/tasks/' + id + '/pause', { method: 'POST' }); loadTasks(); showToast('أوقفت المهمة (الجزئي محفوظ)'); }
  catch (e) { showToast('⛔ ' + e.message); }
};

async function analyze() {
  const url = $('#urlInp').value.trim();
  if (!url) { showToast('الصق رابطًا أولًا'); return; }
  switchTab('video');
  const box = $('#analyzeBox');
  box.innerHTML = '<em>جارٍ التحليل…</em>';
  try {
    const a = await api('/api/analyze', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url })
    });
    const kinds = { file: 'ملف مباشر', video: 'فيديو', torrent: 'تورنت', page: 'صفحة ويب' };
    const kind = kinds[a.kind] || a.kind;
    const optBtns = (a.options || []).length
      ? `<div class="optrow" style="display:flex;gap:6px;flex-wrap:wrap;margin-top:8px">${
          a.options.map(o => `<button class="btn sm" onclick='queueTask(${JSON.stringify(a.url)},"${o}")'>${esc(o)}</button>`).join('')}</div>` : '';
    const addBtn = a.kind === 'file'
      ? `<button class="btn primary sm" style="margin-top:8px" onclick='queueTask(${JSON.stringify(a.url)},${$('#segSel').value})'>⬇ أضف للطابور (${$('#segSel').value} جزء)</button>`
      : '';
    box.innerHTML = `<div class="card">
      <b>${esc(a.platform || kind)}</b> <span class="muted">${kind}</span><br>
      <small class="muted" dir="ltr">${esc(a.content_type || '')} · ${a.size_bytes ? fmtB(a.size_bytes) : 'حجم غير معلن'} · ${a.accept_ranges ? '✅ تقسيم مدعوم' : '⛔ تدفق واحد'}</small><br>
      <small class="muted" dir="ltr">${esc(a.url)}</small>
      ${a.notes ? `<p class="muted">${esc(a.notes)}</p>` : ''}
      ${optBtns}${addBtn}
    </div>`;
  } catch (e) {
    box.innerHTML = `<div class="card warn">⛔ ${esc(e.message)}</div>`;
  }
}

window.queueTask = async (url, seg) => {
  try {
    await api('/api/tasks?url=' + encodeURIComponent(url) + '&segments=' + encodeURIComponent(seg), { method: 'POST' });
    showToast('أُضيفت للطابور ✅');
    switchTab('downloads');
    loadTasks();
  } catch (e) { showToast('⛔ ' + e.message); }
};

async function loadLogs() {
  try {
    const d = await api('/api/logs?tab=' + logTab);
    const ev = d.events || [];
    const rows = ev.slice(-120).map(e => {
      const c = e.level === 'error' ? 'err' : (e.level === 'success' ? 'ok' : '');
      return `<div class="row ${c}">[${new Date(e.ts).toLocaleTimeString('ar-EG', { hour12: false })}] ${esc(e.text)}</div>`;
    }).join('');
    $('#logsBox').innerHTML = rows || '<span class="muted">لا أحداث بعد</span>';
    if (ev.length) $('#miniLog').innerHTML = ev.slice(-6).map(e => `<div>${esc(e.text)}</div>`).join('');
  } catch { /* تجاهل هادئ */ }
}

$$('.tab').forEach(b => b.onclick = () => {
  $$('.tab').forEach(x => x.classList.remove('active'));
  b.classList.add('active');
  logTab = b.dataset.l;
  loadLogs();
});

async function loadCaps() {
  try {
    capsCache = await api('/api/capabilities');
    $('#ytNote').textContent = capsCache.yt_dlp
      ? '✅ yt-dlp متوفر — منصات الفيديو تعمل عبر الجسر.'
      : '⛔ yt-dlp غير مثبت — ثبّته لتفعيل منصات الفيديو (شفافية كاملة لا وهم).';
    $('#torrentCap').innerHTML = capsCache.torrent_bridge
      ? '<div class="card ok">جسر التورنت متوفر — v0.2 يوصل.</div>'
      : '<div class="muted">لا aria2/transmission — التورنت في v0.2.</div>';
    $('#toolCaps').innerHTML = capsCache.ffmpeg
      ? '✅ ffmpeg متوفر — استخراج الصوت جاهز.'
      : '⛔ ffmpeg غير مثبت — التقسيم/الدمج/الضغط تعمل داخليًا وطنيةً؛ الصوت يحتاج ffmpeg.';
  } catch { /* تجاهل هادئ */ }
}

window.toolOp = async (op) => {
  const body = op === 'split' ? { src: $('#splitSrc').value, parts: +($('#splitN').value || 8), dest: '' }
    : op === 'join' ? { src: '', parts_list: ($('#joinList').value || '').split(',').map(s => s.trim()).filter(Boolean), dest: $('#joinOut').value || '' }
    : op === 'zip' ? { src: $('#zipSrc').value, dest: $('#zipOut').value || '' }
    : { src: $('#audioSrc').value, dest: $('#audioOut').value || '' };
  if ((op === 'join' && body.parts_list.length < 2) || (op !== 'join' && !body.src)) { showToast('املأ الحقول أولًا'); return; }
  showToast('جارٍ التنفيذ…');
  try {
    const r = await api('/api/tools/' + op, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body)
    });
    r.ok ? showToast('تمت العملية ✅ ' + op) : showToast('⛔ ' + (r.error || 'فشل'));
  } catch (e) { showToast('⛔ ' + e.message); }
};

async function loadSettings() {
  try {
    const lic = await api('/api/license');
    $('#licenseBox').innerHTML = `<h2>📜 الترخيص</h2>
      <b>${esc(lic.product)}</b> — ${esc(lic.license)} ${lic.active ? '<span class="pill g">مفعّل</span>' : ''}
      <div class="muted">المالك: <b>${esc(lic.owner)}</b> · المفتاح المحلي: <code dir="ltr">${esc(lic.key || '')}</code>
      <br>الدعم: <a class="muted" href="mailto:${esc(lic.support.email)}">${esc(lic.support.email)}</a> · ${esc(lic.support.phone)}</div>`;
    const s = await api('/api/settings');
    $('#settingsBox').innerHTML = `<h2>🧩 الحالة المخزنة</h2>
      <div class="v">${fmtB((await api('/api/stats')).storage_bytes || 0)}</div>
      <div class="muted">مجلد: <code dir="ltr">${esc(s.download_dir)}</code> · أجزاء افتراضية: ${s.segments} · موازية: ${s.max_parallel}</div>`;
  } catch { /* تجاهل هادئ */ }
}

$('#analyzeBtn').onclick = analyze;
$('#urlInp').addEventListener('keydown', (e) => { if (e.key === 'Enter') analyze(); });

loadCaps(); loadSettings(); switchTab('home');
loadStats(); loadTasks(); loadLogs();
setInterval(loadStats, 2000);
setInterval(loadTasks, 1800);
setInterval(loadLogs, 4000);
window.addEventListener('resize', drawNet);
