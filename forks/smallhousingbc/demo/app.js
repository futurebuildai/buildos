// SHBC × BuildOS — demo app
// Vanilla JS, no build step. Loads mock data, hash-routes, renders views.
// See README.md for the demo's mock-data world; DEMO_SCRIPT.md for the tour.

(() => {
  // ============================================================
  // State + bootstrap
  // ============================================================

  const STATE = {
    data: {},               // loaded JSON, keyed by file basename
    currentUserId: 'user-marc',
    today: new Date(),      // re-anchor with ?today=YYYY-MM-DD
    transitions: {},        // simulated state mutations triggered by demo actions
  };

  // Allow demo presenters to pin "today" so the date math is reproducible.
  const params = new URLSearchParams(location.search);
  if (params.get('today')) {
    const t = new Date(params.get('today'));
    if (!Number.isNaN(t.valueOf())) STATE.today = t;
  }
  const todayUTCDateOnly = (() => {
    const d = STATE.today;
    return new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
  })();

  const DATA_FILES = [
    'organizations', 'users', 'projects', 'municipalities', 'design_library',
    'bylaw_checklist', 'procurement', 'invoices', 'certifications', 'leads',
    'feed_cards', 'schedules', 'budgets',
  ];

  async function bootstrap() {
    try {
      await Promise.all(DATA_FILES.map(async (name) => {
        const r = await fetch(`data/${name}.json`);
        if (!r.ok) throw new Error(`load ${name}: ${r.status}`);
        STATE.data[name] = await r.json();
      }));
    } catch (e) {
      document.getElementById('main').innerHTML = `
        <div class="card">
          <h2 class="page-title">Mock data won't load via file://</h2>
          <p class="muted">Run a tiny local server first:</p>
          <pre class="mono">cd demo &amp;&amp; python3 -m http.server 8765</pre>
          <p class="muted">Then open <a href="http://localhost:8765">http://localhost:8765</a>.</p>
          <p class="mono muted">${e.message}</p>
        </div>`;
      return;
    }
    populateUserSwitcher();
    window.addEventListener('hashchange', router);
    document.getElementById('user-switcher').addEventListener('change', (e) => {
      STATE.currentUserId = e.target.value;
      router();
    });
    if (!location.hash) location.hash = '#/feed';
    router();
  }

  // ============================================================
  // Helpers
  // ============================================================

  const $ = (sel) => document.querySelector(sel);

  function fmtMoney(cents, currencyCode) {
    const dollars = cents / 100;
    return new Intl.NumberFormat('en-CA', {
      style: 'currency', currency: currencyCode || 'CAD',
      minimumFractionDigits: 0, maximumFractionDigits: 0,
    }).format(dollars);
  }

  function offsetDateUTC(days) {
    return new Date(todayUTCDateOnly.getTime() + days * 86400000);
  }
  function fmtDate(d) {
    if (!d) return '—';
    if (typeof d === 'string') d = new Date(d);
    return d.toLocaleDateString('en-CA', { weekday: 'short', month: 'short', day: 'numeric' });
  }
  function fmtDateLong(d) {
    if (!d) return '—';
    if (typeof d === 'string') d = new Date(d);
    return d.toLocaleDateString('en-CA', { year: 'numeric', month: 'short', day: 'numeric' });
  }
  function fmtRelative(offsetDays) {
    if (offsetDays === 0) return 'today';
    if (offsetDays === 1) return 'tomorrow';
    if (offsetDays === -1) return 'yesterday';
    if (offsetDays > 0) return `in ${offsetDays} days`;
    return `${Math.abs(offsetDays)} days ago`;
  }
  function daysBetween(aISO, b = todayUTCDateOnly) {
    const a = typeof aISO === 'string' ? new Date(aISO) : aISO;
    return Math.round((a - b) / 86400000);
  }
  function escapeHtml(s) {
    return String(s ?? '').replace(/[&<>"']/g, (c) => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
  }

  function getOrg(id) { return STATE.data.organizations.organizations.find((o) => o.id === id); }
  function getUser(id) { return STATE.data.users.users.find((u) => u.id === id); }
  function getProject(id) { return STATE.data.projects.projects.find((p) => p.id === id); }
  function getMuni(id) { return STATE.data.municipalities.municipalities.find((m) => m.id === id); }
  function getDesign(id) { return STATE.data.design_library.designs.find((d) => d.id === id); }
  function userOrg() { return getOrg(getUser(STATE.currentUserId).org_id); }
  function isShbcAdmin() { return userOrg().kind === 'cohort_operator'; }

  function projectsForCurrentUser() {
    const org = userOrg();
    if (org.kind === 'cohort_operator') return STATE.data.projects.projects;
    return STATE.data.projects.projects.filter((p) => p.org_id === org.id);
  }

  function feedCardsForCurrentUser() {
    return STATE.data.feed_cards.cards.filter((c) =>
      c.user_id === STATE.currentUserId
      || (isShbcAdmin() && c.priority === 'critical')
    );
  }

  function statusLabel(s) {
    return ({
      construction: 'Construction',
      permit_pending: 'Permit pending',
      design: 'Design',
      complete: 'Complete',
    }[s] || s);
  }
  function typologyLabel(t) {
    return ({
      laneway: 'Laneway',
      adu: 'ADU',
      duplex: 'Duplex',
      triplex: 'Triplex',
      fourplex: 'Fourplex',
      sixplex: 'Sixplex',
      multiplex: 'Multiplex',
    }[t] || t);
  }

  function populateUserSwitcher() {
    const sel = document.getElementById('user-switcher');
    const principals = STATE.data.users.users.filter((u) =>
      u.role === 'owner' || u.id === 'user-eastvan-amir'
    );
    sel.innerHTML = principals.map((u) => {
      const o = getOrg(u.org_id);
      return `<option value="${u.id}" ${u.id === STATE.currentUserId ? 'selected' : ''}>
        ${escapeHtml(u.first_name)} ${escapeHtml(u.last_name)} — ${escapeHtml(o.name)}
      </option>`;
    }).join('');
  }

  function setActiveNav(routeKey) {
    document.querySelectorAll('.nav a').forEach((a) => {
      a.classList.toggle('active', a.dataset.route === routeKey);
    });
  }

  function setFeedBadge() {
    const n = feedCardsForCurrentUser().filter((c) =>
      c.priority === 'critical' || c.priority === 'urgent'
    ).length;
    document.getElementById('badge-feed').textContent = n;
  }

  // ============================================================
  // Router
  // ============================================================

  function router() {
    const hash = location.hash.replace(/^#/, '') || '/feed';
    const main = document.getElementById('main');
    const parts = hash.split('/').filter(Boolean);
    const root = parts[0] || 'feed';
    setActiveNav(root);
    setFeedBadge();

    try {
      if (root === 'feed') main.innerHTML = viewFeed();
      else if (root === 'projects' && parts.length === 1) main.innerHTML = viewProjects();
      else if (root === 'projects' && parts.length >= 2) main.innerHTML = viewProjectDetail(parts[1], parts[2] || 'overview');
      else if (root === 'pipeline') main.innerHTML = viewPipeline();
      else if (root === 'directory') main.innerHTML = viewDirectory();
      else if (root === 'designs') main.innerHTML = viewDesigns();
      else if (root === 'compliance') main.innerHTML = viewCompliance();
      else main.innerHTML = `<div class="card">Unknown route: ${escapeHtml(hash)}</div>`;
    } catch (e) {
      main.innerHTML = `<div class="card"><strong>Render error</strong><pre class="mono">${escapeHtml(e.stack || e.message)}</pre></div>`;
    }
    attachInteractions();
  }

  // ============================================================
  // Views
  // ============================================================

  // --- Daily Focus Feed ---
  function viewFeed() {
    const user = getUser(STATE.currentUserId);
    const org = userOrg();
    const cards = feedCardsForCurrentUser().sort((a, b) =>
      ['critical', 'urgent', 'normal'].indexOf(a.priority) -
      ['critical', 'urgent', 'normal'].indexOf(b.priority)
    );

    return `
      <div class="banner">
        <div class="banner__text">
          <strong>Good morning, ${escapeHtml(user.first_name)}.</strong>
          <span>${cards.length} cards in your daily focus across ${org.active_projects || projectsForCurrentUser().length} projects. Sorted by priority.</span>
        </div>
        <a href="#/projects" class="btn btn-accent">Open project portfolio →</a>
      </div>

      <h1 class="page-title">Daily Focus</h1>
      <p class="page-subtitle">Critical · urgent · normal — top of stack first.</p>

      <div class="feed-stack">
        ${cards.map(renderFeedCard).join('') || '<div class="card muted">No cards.</div>'}
      </div>
    `;
  }

  function renderFeedCard(c) {
    const project = c.project_id ? getProject(c.project_id) : null;
    const projLink = project
      ? `<a href="#/projects/${project.id}">${escapeHtml(project.name)}</a> · ${escapeHtml(getMuni(project.muni_id).name)}`
      : 'Cross-project';
    const created = fmtRelative(c.created_offset_days);
    const actions = (c.actions || []).map((a) =>
      `<button class="btn ${a.action_type.startsWith('permit.approved') ? 'btn-primary' : ''}"
         data-card-id="${c.id}" data-action-type="${escapeHtml(a.action_type)}">
         ${escapeHtml(a.label)}
       </button>`
    ).join('');

    return `
      <div class="feed-card priority-${c.priority}">
        <div class="feed-card__head">
          <div class="feed-card__title">${escapeHtml(c.title)}</div>
          <span class="pill ${c.priority === 'critical' ? 'critical' : c.priority === 'urgent' ? 'warning' : 'ok'}">${escapeHtml(c.priority)}</span>
        </div>
        <div class="feed-card__body">${escapeHtml(c.body)}</div>
        <div class="feed-card__actions">${actions}</div>
        <div class="feed-card__meta">${projLink} · ${created}</div>
      </div>
    `;
  }

  // --- Project Portfolio ---
  function viewProjects() {
    const projects = projectsForCurrentUser();

    const kpis = `
      <div class="kpi-row">
        <div class="kpi"><div class="kpi__label">Active projects</div><div class="kpi__value">${projects.length}</div><div class="kpi__delta">across ${new Set(projects.map(p => p.muni_id)).size} municipalities</div></div>
        <div class="kpi"><div class="kpi__label">Total budget</div><div class="kpi__value">${fmtMoney(projects.reduce((s, p) => s + p.budget_cents, 0), 'CAD')}</div><div class="kpi__delta">CAD across ${projects.length} projects</div></div>
        <div class="kpi"><div class="kpi__label">At-risk</div><div class="kpi__value">${projects.filter(p => p.schedule_health === 'AT_RISK').length}</div><div class="kpi__delta">flagged by physics engine</div></div>
        <div class="kpi"><div class="kpi__label">From SHBC Toolbox</div><div class="kpi__value">${projects.filter(p => p.homeowner_referral_source === 'shbc_toolbox').length}</div><div class="kpi__delta">homeowner-originated</div></div>
      </div>
    `;

    return `
      <h1 class="page-title">Project Portfolio</h1>
      <p class="page-subtitle">${escapeHtml(userOrg().name)} · ${projects.length} active SSMUH projects</p>
      ${kpis}
      <div class="filter-bar">
        <label>Typology</label>
        <button class="chip active">All</button>
        ${['laneway','adu','duplex','triplex','fourplex','sixplex'].map(t => `<button class="chip">${typologyLabel(t)}</button>`).join('')}
      </div>

      <div class="portfolio">
        ${projects.map(renderProjectCard).join('')}
      </div>
    `;
  }

  function renderProjectCard(p) {
    const muni = getMuni(p.muni_id);
    const org = getOrg(p.org_id);
    const healthPill = p.schedule_health === 'AT_RISK'
      ? `<span class="pill warning">At risk</span>`
      : `<span class="pill ok">On track</span>`;
    return `
      <div class="project-card" onclick="location.hash='#/projects/${p.id}'">
        <div class="project-card__name">${escapeHtml(p.name)}</div>
        <div class="project-card__addr">${escapeHtml(p.address)}</div>
        <div class="project-card__row"><span>Builder</span><span>${escapeHtml(org.name)}</span></div>
        <div class="project-card__row"><span>Typology</span><span>${typologyLabel(p.typology)} · ${p.gsf.toLocaleString()} sf</span></div>
        <div class="project-card__row"><span>Budget</span><span class="mono">${fmtMoney(p.budget_cents, p.currency_code)}</span></div>
        <div class="project-card__row"><span>Phase</span><span>${escapeHtml(p.phase)}</span></div>
        <div class="project-card__row"><span>Status</span><span>${healthPill}</span></div>
      </div>
    `;
  }

  // --- Project Detail ---
  function viewProjectDetail(projectId, tab) {
    const p = getProject(projectId);
    if (!p) return `<div class="card">Project not found: ${escapeHtml(projectId)}</div>`;
    const muni = getMuni(p.muni_id);
    const org = getOrg(p.org_id);
    const design = getDesign(p.design_id);

    const tabs = ['overview', 'schedule', 'budget', 'procurement', 'bylaw', 'design', 'crew']
      .map((t) => `<a href="#/projects/${projectId}/${t}" class="${t === tab ? 'active' : ''}">${t.charAt(0).toUpperCase() + t.slice(1)}</a>`).join('');

    let body = '';
    if (tab === 'overview')   body = renderProjectOverview(p, muni, org, design);
    if (tab === 'schedule')   body = renderProjectSchedule(p);
    if (tab === 'budget')     body = renderProjectBudget(p);
    if (tab === 'procurement') body = renderProjectProcurement(p);
    if (tab === 'bylaw')      body = renderProjectBylaw(p, muni);
    if (tab === 'design')     body = renderProjectDesign(p, design);
    if (tab === 'crew')       body = renderProjectCrew(p, org);

    return `
      <div class="project-head">
        <div>
          <h1 class="page-title">${escapeHtml(p.name)}</h1>
          <div class="project-head__meta">
            <span>${escapeHtml(p.address)}</span>
            <span>·</span>
            <span><strong>${escapeHtml(org.name)}</strong></span>
            <span>·</span>
            <span>${typologyLabel(p.typology)}</span>
            <span>·</span>
            <span>${p.gsf.toLocaleString()} gsf</span>
          </div>
        </div>
        <div>
          <span class="pill ${p.schedule_health === 'AT_RISK' ? 'warning' : 'ok'}">${p.schedule_health.replace('_', ' ').toLowerCase()}</span>
        </div>
      </div>
      <div class="tabs">${tabs}</div>
      ${body}
    `;
  }

  function renderProjectOverview(p, muni, org, design) {
    return `
      <div class="kpi-row">
        <div class="kpi"><div class="kpi__label">Budget</div><div class="kpi__value">${fmtMoney(p.budget_cents, p.currency_code)}</div><div class="kpi__delta">${p.currency_code}</div></div>
        <div class="kpi"><div class="kpi__label">Start</div><div class="kpi__value" style="font-size:18px">${fmtDateLong(p.start_date)}</div></div>
        <div class="kpi"><div class="kpi__label">Target occupancy</div><div class="kpi__value" style="font-size:18px">${fmtDateLong(p.target_occupancy_date)}</div></div>
        <div class="kpi"><div class="kpi__label">Permit status</div><div class="kpi__value" style="font-size:18px">${escapeHtml(p.permit_status.replace(/_/g, ' '))}</div></div>
      </div>
      <div class="card">
        <h3 style="margin:0 0 8px">Site &amp; design</h3>
        <table class="table">
          <tbody>
            <tr><td>Municipality</td><td><strong>${escapeHtml(muni.name)}</strong> — ${escapeHtml(muni.ssmuh_zone_examples.join(', '))}</td></tr>
            <tr><td>Site zoning</td><td class="mono">${escapeHtml(p.site_zoning)}</td></tr>
            <tr><td>Lot size</td><td>${p.lot_size_sqm.toLocaleString()} m²</td></tr>
            <tr><td>Design</td><td><a href="#/designs">${escapeHtml(design.name)}</a> · <span class="mono">${escapeHtml(design.code)}</span> (${escapeHtml(design.source)})</td></tr>
            <tr><td>Lead source</td><td>${p.homeowner_referral_source === 'shbc_toolbox' ? 'SHBC Toolbox' : p.homeowner_referral_source === 'shbc_workshop' ? 'SHBC Workshop' : p.homeowner_referral_source === 'shbc_pre_design_review' ? 'SHBC Pre-Design Review' : 'Direct'}</td></tr>
            ${p.notes ? `<tr><td>Notes</td><td>${escapeHtml(p.notes)}</td></tr>` : ''}
          </tbody>
        </table>
      </div>
    `;
  }

  function renderProjectSchedule(p) {
    const sched = STATE.data.schedules.schedules.find((s) => s.project_id === p.id);
    if (!sched) return `<div class="card muted">No schedule defined for this project yet.</div>`;
    const tasks = sched.tasks;
    const minOff = Math.min(...tasks.map((t) => t.start_offset_days));
    const maxOff = Math.max(...tasks.map((t) => t.start_offset_days + t.duration_days + (t.weather_adjusted_days || 0)));
    const totalDays = maxOff - minOff;
    const todayPct = ((0 - minOff) / totalDays) * 100;

    return `
      <p class="muted" style="margin:0 0 14px">CPM with weather-adjusted spans (BuildOS SWIM model). Critical path in red. Today's date marker shows where construction stands relative to the baseline.</p>
      <div class="gantt">
        ${tasks.map((t) => {
          const startPct = ((t.start_offset_days - minOff) / totalDays) * 100;
          const widthPct = ((t.duration_days + (t.weather_adjusted_days || 0)) / totalDays) * 100;
          const cls = [t.is_critical ? 'critical' : '', t.status].filter(Boolean).join(' ');
          return `
            <div class="gantt-row">
              <div class="gantt-row__label">
                <span>${escapeHtml(t.name)}</span>
                <span class="gantt-row__sub">${escapeHtml(t.wbs_code)} · ${t.duration_days}d${t.weather_adjusted_days ? ` (+${t.weather_adjusted_days}d weather)` : ''} · ${escapeHtml(t.status)}</span>
              </div>
              <div class="gantt-row__bar">
                <div class="today" style="left:${todayPct}%"></div>
                <div class="gantt-bar ${cls}" style="left:${startPct}%; width:${widthPct}%">
                  ${escapeHtml(t.name.length > 28 ? t.name.slice(0, 27) + '…' : t.name)}
                </div>
              </div>
            </div>`;
        }).join('')}
      </div>
      <p class="muted" style="margin-top:12px;font-size:12px">
        <span class="pill critical" style="vertical-align:middle">Critical path</span>
        <span class="pill ok" style="vertical-align:middle;margin-left:6px">On track</span>
        <span class="pill warning" style="vertical-align:middle;margin-left:6px">In progress</span>
        <span class="pill blocked" style="vertical-align:middle;margin-left:6px">Blocked</span>
      </p>
    `;
  }

  function renderProjectBudget(p) {
    const budget = STATE.data.budgets.budgets.find((b) => b.project_id === p.id);
    if (!budget) return `<div class="card muted">No budget detail for this project yet.</div>`;
    const total = budget.lines.reduce((s, l) => s + l.budgeted_cents, 0);
    const committed = budget.lines.reduce((s, l) => s + l.committed_cents, 0);
    const actual = budget.lines.reduce((s, l) => s + l.actual_cents, 0);

    const invoices = STATE.data.invoices.invoices.filter((i) => i.project_id === p.id);

    return `
      <div class="kpi-row">
        <div class="kpi"><div class="kpi__label">Budgeted</div><div class="kpi__value">${fmtMoney(total, budget.currency_code)}</div><div class="kpi__delta">${budget.currency_code}</div></div>
        <div class="kpi"><div class="kpi__label">Committed</div><div class="kpi__value">${fmtMoney(committed, budget.currency_code)}</div><div class="kpi__delta">${Math.round((committed / total) * 100)}%</div></div>
        <div class="kpi"><div class="kpi__label">Actual paid</div><div class="kpi__value">${fmtMoney(actual, budget.currency_code)}</div><div class="kpi__delta">${Math.round((actual / total) * 100)}%</div></div>
        <div class="kpi"><div class="kpi__label">Remaining</div><div class="kpi__value">${fmtMoney(total - committed, budget.currency_code)}</div><div class="kpi__delta">free to commit</div></div>
      </div>
      <h3 style="margin:24px 0 8px">WBS lines</h3>
      <table class="table">
        <thead><tr><th>WBS</th><th>Division</th><th class="num">Budget</th><th class="num">Committed</th><th class="num">Actual</th></tr></thead>
        <tbody>
          ${budget.lines.map((l) => `
            <tr>
              <td class="mono">${escapeHtml(l.wbs_code)}</td>
              <td>${escapeHtml(l.label)}</td>
              <td class="num">${fmtMoney(l.budgeted_cents, budget.currency_code)}</td>
              <td class="num">${fmtMoney(l.committed_cents, budget.currency_code)}</td>
              <td class="num">${fmtMoney(l.actual_cents, budget.currency_code)}</td>
            </tr>`).join('')}
        </tbody>
      </table>
      <h3 style="margin:24px 0 8px">Invoices (${invoices.length})</h3>
      <table class="table">
        <thead><tr><th>Vendor</th><th>Invoice #</th><th>Status</th><th class="num">Amount</th><th class="num">Due / Paid</th></tr></thead>
        <tbody>
          ${invoices.map((i) => {
            const dateOff = i.due_offset_days != null ? `due ${fmtRelative(i.due_offset_days)}` : `paid ${fmtRelative(i.paid_offset_days)}`;
            const pillCls = i.status === 'paid' ? 'ok' : i.status === 'approved' ? 'info' : 'warning';
            return `<tr>
              <td>${escapeHtml(i.vendor)}</td>
              <td class="mono">${escapeHtml(i.invoice_number)}</td>
              <td><span class="pill ${pillCls}">${escapeHtml(i.status)}</span></td>
              <td class="num">${fmtMoney(i.amount_cents, i.currency_code)}</td>
              <td class="num">${dateOff}</td>
            </tr>`;
          }).join('')}
        </tbody>
      </table>
    `;
  }

  function renderProjectProcurement(p) {
    const items = STATE.data.procurement.items
      .filter((i) => i.project_id === p.id)
      .sort((a, b) => ['CRITICAL','WARNING','OK','ORDERED'].indexOf(a.status) - ['CRITICAL','WARNING','OK','ORDERED'].indexOf(b.status));
    if (items.length === 0) return `<div class="card muted">No procurement items yet.</div>`;
    return `
      <p class="muted" style="margin:0 0 14px">ProcurementCheckWorker runs nightly — flips OK / WARNING / CRITICAL based on each row's must-order date. ORDERED rows are terminal.</p>
      <table class="table">
        <thead><tr><th>Status</th><th>Item</th><th>Vendor</th><th class="num">Cost</th><th class="num">Lead + buffer</th><th class="num">Need by / ordered</th></tr></thead>
        <tbody>
          ${items.map((i) => {
            const pillCls = i.status === 'CRITICAL' ? 'critical' : i.status === 'WARNING' ? 'warning' : i.status === 'ORDERED' ? 'ordered' : 'ok';
            const needBy = i.status === 'ORDERED'
              ? `<span class="muted">PO ${escapeHtml(i.po_number || '—')} · ${fmtRelative(daysBetween(i.ordered_at))}</span>`
              : `${fmtRelative(i.need_by_offset_days)}`;
            return `<tr>
              <td><span class="pill ${pillCls}">${escapeHtml(i.status)}</span></td>
              <td>${escapeHtml(i.name)}<br><span class="mono muted">${escapeHtml(i.wbs_code)}</span> ${i.notes ? `<br><span class="muted" style="font-size:11px">${escapeHtml(i.notes)}</span>` : ''}</td>
              <td>${escapeHtml(i.vendor)}</td>
              <td class="num">${fmtMoney(i.estimated_cost_cents, i.currency_code)}</td>
              <td class="num">${i.lead_time_days}d + ${i.weather_buffer_days}d</td>
              <td class="num">${needBy}</td>
            </tr>`;
          }).join('')}
        </tbody>
      </table>
    `;
  }

  function renderProjectBylaw(p, muni) {
    const cl = STATE.data.bylaw_checklist.checklists.find((c) => c.project_id === p.id);
    if (!cl) return `<div class="card muted">No checklist seeded for this project yet.</div>`;
    const total = cl.items.length;
    const done = cl.items.filter((i) => i.status === 'complete' || i.status === 'not_applicable').length;
    const pct = Math.round((done / total) * 100);

    return `
      <div class="banner wood">
        <div class="banner__text">
          <strong>${escapeHtml(muni.name)} — SSMUH Compliance</strong>
          <span>${done}/${total} complete · ${pct}% · Bill 44 + Bill 25 + ${escapeHtml(muni.name)}-specific bylaws.</span>
        </div>
        <div style="font-size:13px;text-align:right">
          ${escapeHtml(muni.bill_44_status)}<br>
          ${escapeHtml(muni.bill_25_status)}
        </div>
      </div>

      <h3 style="margin:0 0 10px">Compliance items</h3>
      <div class="checklist">
        ${cl.items.map((i) => {
          const symbol = i.status === 'complete' ? '✓' : i.status === 'in_progress' ? '⋯' : i.status === 'not_applicable' ? 'N/A' : '○';
          const meta = i.completed_on ? `Completed ${fmtDateLong(i.completed_on)}`
            : i.deadline_offset_days != null ? `Deadline ${fmtRelative(i.deadline_offset_days)}`
            : i.status === 'in_progress' ? 'In progress' : 'Pending';
          const action = i.status === 'in_progress' && i.id === 'blw-cd-3'
            ? `<button class="btn btn-primary" data-action="permit-approve" data-project-id="${p.id}">Mark approved</button>`
            : '';
          return `
            <div class="checklist-item ${i.status}">
              <div class="check">${symbol}</div>
              <div>
                <div class="checklist-item__label">${escapeHtml(i.label)}</div>
                <div class="checklist-item__note">${escapeHtml(meta)}${i.note ? ' · ' + escapeHtml(i.note) : ''}</div>
              </div>
              <div class="checklist-item__action">${action}</div>
            </div>`;
        }).join('')}
      </div>

      <h3 style="margin:24px 0 10px">${escapeHtml(muni.name)} bylaw quirks</h3>
      <div class="card">
        <ul style="margin:0;padding-left:18px;font-size:13px">
          ${muni.quirks.map((q) => `<li style="margin:4px 0">${escapeHtml(q)}</li>`).join('')}
        </ul>
      </div>
    `;
  }

  function renderProjectDesign(p, design) {
    const designs = STATE.data.design_library.designs.filter((d) => d.typology === p.typology);
    return `
      <div class="card" style="margin-bottom:14px">
        <div style="display:flex;justify-content:space-between;align-items:center;gap:14px">
          <div>
            <div class="design-card__source">${escapeHtml(design.source)}</div>
            <h3 style="margin:4px 0">${escapeHtml(design.name)}</h3>
            <span class="mono muted">${escapeHtml(design.code)} · ${design.approx_gsf.toLocaleString()} gsf · ${design.stories}-storey · ${design.units} unit(s)</span>
          </div>
          <a class="btn" target="_blank" href="${escapeHtml(design.external_url)}">Open in source ↗</a>
        </div>
      </div>

      <h3 style="margin:0 0 10px">Other ${typologyLabel(p.typology)} designs in the library</h3>
      <div class="design-grid">
        ${designs.filter((d) => d.id !== design.id).map(renderDesignCard).join('')}
      </div>
    `;
  }

  function renderDesignCard(d) {
    return `
      <div class="design-card">
        <div class="design-card__source">${escapeHtml(d.source)}</div>
        <div class="design-card__name">${escapeHtml(d.name)}</div>
        <div class="design-card__code">${escapeHtml(d.code)}</div>
        <div class="design-card__specs">
          <span>Typology</span><span>${typologyLabel(d.typology)}</span>
          <span>Size</span><span>${d.approx_gsf.toLocaleString()} gsf</span>
          <span>Stories</span><span>${d.stories}</span>
          <span>Units</span><span>${d.units}</span>
          <span>Min lot</span><span>${d.lot_min_sqm} m²</span>
          <span>Cost / sf</span><span class="mono">$${d.approx_cost_per_sf_cad} CAD</span>
        </div>
        <div class="design-card__tags">${d.tags.map((t) => `<span class="design-card__tag">${escapeHtml(t)}</span>`).join('')}</div>
      </div>
    `;
  }

  function renderProjectCrew(p, org) {
    const crew = STATE.data.users.users.filter((u) => u.org_id === org.id);
    const certs = STATE.data.certifications.certifications.filter((c) => crew.some((u) => u.id === c.user_id));
    return `
      <h3 style="margin:0 0 10px">Crew assigned</h3>
      <table class="table" style="margin-bottom:18px">
        <thead><tr><th>Name</th><th>Role</th><th>Title</th></tr></thead>
        <tbody>
          ${crew.map((u) => `<tr><td><strong>${escapeHtml(u.first_name)} ${escapeHtml(u.last_name)}</strong></td><td>${escapeHtml(u.role)}</td><td>${escapeHtml(u.title || '—')}</td></tr>`).join('')}
        </tbody>
      </table>
      <h3 style="margin:0 0 10px">Certifications</h3>
      <table class="table">
        <thead><tr><th>Person</th><th>Cert</th><th class="mono">Number</th><th>Status</th><th>Expires</th></tr></thead>
        <tbody>
          ${certs.map((c) => {
            const u = getUser(c.user_id);
            const expSoon = c.expiry_offset_days < 60 && c.status === 'active';
            const pillCls = c.status === 'expired' ? 'critical' : expSoon ? 'warning' : 'ok';
            return `<tr>
              <td>${escapeHtml(u.first_name)} ${escapeHtml(u.last_name)}</td>
              <td>${escapeHtml(c.cert_type)}</td>
              <td class="mono">${escapeHtml(c.cert_number)}</td>
              <td><span class="pill ${pillCls}">${escapeHtml(c.status)}${expSoon ? ' · expiring' : ''}</span></td>
              <td>${c.expiry_offset_days >= 9999 ? 'No expiry' : fmtRelative(c.expiry_offset_days)}</td>
            </tr>`;
          }).join('')}
        </tbody>
      </table>
    `;
  }

  // --- Pipeline ---
  function viewPipeline() {
    const leads = STATE.data.leads.leads.filter((l) => {
      if (isShbcAdmin()) return true;
      return l.matched_builder_org_id === userOrg().id;
    });

    return `
      <h1 class="page-title">Pipeline</h1>
      <p class="page-subtitle">Homeowner inquiries from the SHBC Toolbox + workshop intake. Matched to ${escapeHtml(userOrg().name)} on typology + municipality.</p>

      <div class="kpi-row">
        <div class="kpi"><div class="kpi__label">Active leads</div><div class="kpi__value">${leads.length}</div></div>
        <div class="kpi"><div class="kpi__label">Toolbox-sourced</div><div class="kpi__value">${leads.filter(l => l.source === 'shbc_toolbox').length}</div></div>
        <div class="kpi"><div class="kpi__label">Estimated GMV</div><div class="kpi__value">${fmtMoney(leads.reduce((s,l) => s + l.estimated_budget_cents, 0), 'CAD')}</div></div>
      </div>

      ${leads.map(renderLeadCard).join('') || '<div class="card muted">No active leads matched to your firm.</div>'}
    `;
  }

  function renderLeadCard(l) {
    const muni = getMuni(l.muni_id);
    const builder = getOrg(l.matched_builder_org_id);
    const design = getDesign(l.design_id_picked);
    const stagePill = ({
      lead_captured: 'warning',
      pre_design_booked: 'info',
      feasibility_review: 'info',
    })[l.stage] || 'info';

    return `
      <div class="lead-card">
        <div class="lead-card__top">
          <div>
            <div class="lead-card__name">${escapeHtml(l.lead_name)}</div>
            <div class="lead-card__meta">${escapeHtml(l.address)} · ${fmtRelative(l.captured_offset_days)} via ${escapeHtml(l.source.replace('shbc_', 'SHBC '))}</div>
          </div>
          <span class="pill ${stagePill}">${escapeHtml(l.stage.replace(/_/g, ' '))}</span>
        </div>
        <div class="lead-card__details">
          <div><span>Typology</span><strong>${typologyLabel(l.interested_typology)}</strong></div>
          <div><span>Municipality</span><strong>${escapeHtml(muni.name)}</strong></div>
          <div><span>Estimated budget</span><strong class="mono">${fmtMoney(l.estimated_budget_cents, l.currency_code)}</strong></div>
          <div><span>Design picked</span><strong>${escapeHtml(design.name)}</strong></div>
          <div><span>GSF</span><strong>${l.estimated_gsf.toLocaleString()}</strong></div>
          <div><span>Matched builder</span><strong>${escapeHtml(builder.name)}</strong></div>
        </div>
        <div class="lead-card__meta"><em>Match reason:</em> ${escapeHtml(l.match_reason)}</div>
        ${l.stage === 'lead_captured' ? `
          <div class="feed-card__actions" style="margin-top:10px">
            <button class="btn btn-primary" data-action="lead-advance" data-lead-id="${l.id}">Schedule pre-design review</button>
            <button class="btn">Decline</button>
          </div>` : ''}
      </div>
    `;
  }

  // --- Cohort Directory ---
  function viewDirectory() {
    const orgs = STATE.data.organizations.organizations.filter((o) => o.kind === 'builder');
    return `
      <h1 class="page-title">Cohort Directory</h1>
      <p class="page-subtitle">SHBC Multiplex Cohort 2026.01 · ${orgs.length} firms. Sort: rotational + capacity-aware (Smallworks-COI-safe — see governance MOU).</p>
      <div class="filter-bar">
        <label>Filter</label>
        <button class="chip active">All firms</button>
        <button class="chip">Available for sub-trade</button>
        <button class="chip">Same municipality</button>
      </div>
      <div class="directory-grid">
        ${orgs.map(renderDirectoryCard).join('')}
      </div>
    `;
  }

  function renderDirectoryCard(o) {
    const principal = getUser(o.principal_user_id);
    const capCls = o.capacity_pct >= 90 ? 'full' : o.capacity_pct >= 75 ? 'high' : '';
    return `
      <div class="directory-card">
        <div class="directory-card__name">${escapeHtml(o.name)}</div>
        <div class="directory-card__city">${escapeHtml(o.city)}, BC · est. ${o.established} · crew of ${o.crew_size}</div>
        <div class="directory-card__specialty"><strong>Specialty:</strong> ${escapeHtml(o.specialty)}</div>
        <div class="directory-card__specialty muted">Principal: ${principal ? escapeHtml(principal.first_name + ' ' + principal.last_name) : '—'}</div>
        <div class="directory-card__capacity">
          Capacity allocated · ${o.capacity_pct}%
          <div class="capacity-bar"><div class="capacity-bar__fill ${capCls}" style="width:${o.capacity_pct}%"></div></div>
        </div>
        <div class="directory-card__badges">
          <span class="pill info">${escapeHtml(o.training_cohort)}</span>
          <span class="pill ${o.available_for_subtrade ? 'ok' : 'warning'}">${o.available_for_subtrade ? 'Available for sub-trade' : 'Fully booked'}</span>
          <span class="pill">${o.active_projects} active</span>
        </div>
      </div>
    `;
  }

  // --- Design Library ---
  function viewDesigns() {
    const designs = STATE.data.design_library.designs;
    const grouped = ['SHBC Toolbox', 'CMHC HDC 2025', 'BC Standardized 2024'];
    return `
      <h1 class="page-title">Design Library</h1>
      <p class="page-subtitle">${designs.length} pre-reviewed SSMUH designs · curated from CMHC, the Province of BC, and SHBC Toolbox. Each card opens the source in a new tab.</p>
      <div class="filter-bar">
        <label>Typology</label>
        <button class="chip active">All</button>
        ${['laneway','adu','duplex','triplex','fourplex','sixplex'].map(t => `<button class="chip">${typologyLabel(t)}</button>`).join('')}
      </div>
      ${grouped.map((src) => `
        <h3 style="margin:24px 0 10px">${escapeHtml(src)}</h3>
        <div class="design-grid">
          ${designs.filter((d) => d.source === src).map(renderDesignCard).join('')}
        </div>
      `).join('')}
    `;
  }

  // --- Compliance / Bill 25 ---
  function viewCompliance() {
    const projects = STATE.data.projects.projects;
    const byMuni = {};
    const byTypology = {};
    projects.forEach((p) => {
      byMuni[p.muni_id] = (byMuni[p.muni_id] || 0) + 1;
      byTypology[p.typology] = (byTypology[p.typology] || 0) + 1;
    });

    return `
      <h1 class="page-title">Bill 25 Compliance Reporting</h1>
      <p class="page-subtitle">Province of BC mandate (Nov 2025) — municipalities must report SSMUH housing-target progress. The cohort's combined picture below is one CSV away from feeding a city's report.</p>

      <div class="kpi-row">
        <div class="kpi"><div class="kpi__label">Active SSMUH projects</div><div class="kpi__value">${projects.length}</div><div class="kpi__delta">across the cohort</div></div>
        <div class="kpi"><div class="kpi__label">Units in delivery</div><div class="kpi__value">${projects.reduce((s,p)=>s+(p.typology==='sixplex'?6:p.typology==='fourplex'?4:p.typology==='triplex'?3:p.typology==='duplex'?2:1),0)}</div><div class="kpi__delta">eligible for Bill 25 reporting</div></div>
        <div class="kpi"><div class="kpi__label">Municipalities</div><div class="kpi__value">${Object.keys(byMuni).length}</div></div>
        <div class="kpi"><div class="kpi__label">Combined budget</div><div class="kpi__value">${fmtMoney(projects.reduce((s,p)=>s+p.budget_cents,0), 'CAD')}</div></div>
      </div>

      <div class="export-card">
        <h2 style="margin:0 0 4px">Export housing-target report</h2>
        <p class="muted" style="margin:0 0 14px">CSV per Bill 25 schema · counts by typology, municipality, month-of-occupancy</p>
        <button class="btn btn-primary" data-action="export-csv">Generate &amp; download CSV</button>
      </div>

      <h3 style="margin:0 0 10px">By municipality</h3>
      <table class="table" style="margin-bottom:18px">
        <thead><tr><th>Municipality</th><th>Bill 44 status</th><th>Bill 25 status</th><th class="num">Projects</th></tr></thead>
        <tbody>
          ${Object.entries(byMuni).map(([id, n]) => {
            const m = getMuni(id);
            return `<tr>
              <td>${escapeHtml(m.name)}</td>
              <td><span class="pill ok">${escapeHtml(m.bill_44_status)}</span></td>
              <td><span class="pill ${m.bill_25_status.startsWith('On') ? 'ok' : 'warning'}">${escapeHtml(m.bill_25_status)}</span></td>
              <td class="num">${n}</td>
            </tr>`;
          }).join('')}
        </tbody>
      </table>

      <h3 style="margin:0 0 10px">By typology</h3>
      <table class="table">
        <thead><tr><th>Typology</th><th class="num">Projects</th><th class="num">Units</th></tr></thead>
        <tbody>
          ${Object.entries(byTypology).map(([t, n]) => {
            const units = (t === 'sixplex' ? 6 : t === 'fourplex' ? 4 : t === 'triplex' ? 3 : t === 'duplex' ? 2 : 1) * n;
            return `<tr><td>${typologyLabel(t)}</td><td class="num">${n}</td><td class="num">${units}</td></tr>`;
          }).join('')}
        </tbody>
      </table>
    `;
  }

  // ============================================================
  // Interactions / state mutations
  // ============================================================

  function attachInteractions() {
    document.querySelectorAll('[data-action]').forEach((el) => {
      el.addEventListener('click', handleAction);
    });
    document.querySelectorAll('[data-action-type]').forEach((el) => {
      el.addEventListener('click', handleFeedAction);
    });
  }

  function handleAction(e) {
    const action = e.currentTarget.dataset.action;
    if (action === 'export-csv') return exportComplianceCSV();
    if (action === 'permit-approve') return approveHeritagePermit(e.currentTarget.dataset.projectId);
    if (action === 'lead-advance') return advanceLead(e.currentTarget.dataset.leadId);
  }

  function handleFeedAction(e) {
    const cardId = e.currentTarget.dataset.cardId;
    const at = e.currentTarget.dataset.actionType;
    const card = STATE.data.feed_cards.cards.find((c) => c.id === cardId);
    if (!card) return;

    // Simulate handling — remove the card and emit a follow-up
    STATE.data.feed_cards.cards = STATE.data.feed_cards.cards.filter((c) => c.id !== cardId);

    if (at === 'permit.approved' && card.project_id) {
      approveHeritagePermit(card.project_id);
      return;
    }
    if (at === 'lead.schedule_review') {
      const lead = STATE.data.leads.leads.find((l) =>
        STATE.data.feed_cards.cards.find((c) => c.id === cardId)?.id === cardId
      );
      // Best effort — match the demo's primary lead
      const target = STATE.data.leads.leads.find((l) => l.id === 'lead-001');
      if (target) target.stage = 'pre_design_booked';
      router();
      flash('Pre-design review scheduled. Lead moved to pre_design_booked stage.');
      return;
    }
    if (at === 'procurement.issue_po') {
      const itemId = (e.currentTarget.dataset || {}).payload || null;
      // Find the item from the card payload
      const items = STATE.data.procurement.items;
      const target = items.find((i) => i.status === 'CRITICAL') || items.find((i) => i.status === 'WARNING');
      if (target) {
        target.status = 'ORDERED';
        target.po_number = 'PO-' + Math.floor(100000 + Math.random() * 900000);
        target.ordered_at = new Date().toISOString().slice(0, 10);
      }
      router();
      flash(`Purchase order issued. Item flipped CRITICAL → ORDERED.`);
      return;
    }
    router();
    flash(`Action acknowledged · ${at}`);
  }

  function approveHeritagePermit(projectId) {
    // Update bylaw checklist + project + emit a feed card.
    const cl = STATE.data.bylaw_checklist.checklists.find((c) => c.project_id === projectId);
    if (cl) {
      const item = cl.items.find((i) => i.id === 'blw-cd-3');
      if (item) {
        item.status = 'complete';
        item.completed_on = new Date().toISOString().slice(0, 10);
        item.note = 'Approved by City of Vancouver Heritage; no conditions attached.';
        delete item.deadline_offset_days;
      }
    }
    const p = getProject(projectId);
    if (p) {
      p.permit_status = 'issued';
      p.schedule_health = 'ON_TRACK';
      if (p.notes) p.notes = 'Heritage Character Review approved; framing rescheduled.';
    }
    // Slide framing tasks forward (simulate CPM recompute)
    const sched = STATE.data.schedules.schedules.find((s) => s.project_id === projectId);
    if (sched) {
      sched.tasks.forEach((t) => {
        if (['t-cd-03', 't-cd-04', 't-cd-05'].includes(t.id)) {
          t.start_offset_days -= 28;
          if (t.id === 't-cd-03') t.status = 'in_progress';
        }
      });
    }
    // Push a follow-up feed card
    STATE.data.feed_cards.cards.unshift({
      id: 'card-followup-' + Date.now(),
      priority: 'normal',
      user_id: STATE.currentUserId,
      project_id: projectId,
      card_type: 'schedule.recomputed',
      title: 'Schedule recovered 4 weeks after heritage approval',
      body: 'Framing tasks slid forward 28 days following the Heritage Character Review approval. Critical path recomputed; Commercial Drive Fourplex back on track.',
      created_offset_days: 0,
      actions: [],
    });
    router();
    flash('Heritage approval applied. Schedule recovered 4 weeks. New focus card emitted.');
  }

  function advanceLead(leadId) {
    const lead = STATE.data.leads.leads.find((l) => l.id === leadId);
    if (lead) lead.stage = 'pre_design_booked';
    router();
    flash('Lead moved to pre_design_booked.');
  }

  function exportComplianceCSV() {
    const projects = STATE.data.projects.projects;
    const rows = [['project_id', 'name', 'org_id', 'muni', 'typology', 'units', 'gsf', 'budget_cents', 'currency', 'permit_status', 'target_occupancy_date']];
    projects.forEach((p) => {
      const muni = getMuni(p.muni_id);
      const units = p.typology === 'sixplex' ? 6 : p.typology === 'fourplex' ? 4 : p.typology === 'triplex' ? 3 : p.typology === 'duplex' ? 2 : 1;
      rows.push([p.id, `"${p.name}"`, p.org_id, muni.name, p.typology, units, p.gsf, p.budget_cents, p.currency_code, p.permit_status, p.target_occupancy_date]);
    });
    const csv = rows.map((r) => r.join(',')).join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'shbc-cohort-bill25-' + new Date().toISOString().slice(0, 10) + '.csv';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    flash('Bill 25 CSV downloaded.');
  }

  function flash(msg) {
    // Styling lives in styles.css (#flash). Single-element toast that
    // gets reused; opacity is the only inline bit we toggle.
    let el = document.getElementById('flash');
    if (!el) {
      el = document.createElement('div');
      el.id = 'flash';
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.style.opacity = '1';
    clearTimeout(el._t);
    el._t = setTimeout(() => { el.style.transition = 'opacity 0.4s'; el.style.opacity = '0'; }, 3500);
  }

  // ============================================================

  bootstrap();
})();
