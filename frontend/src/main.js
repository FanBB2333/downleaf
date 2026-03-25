// Wails runtime bindings are injected automatically at /wails/runtime.js
// and Go bindings at /wails/ipc.js — we call them via window.go.gui.App.*

// Wait for Wails runtime to be ready
window.addEventListener('DOMContentLoaded', init);

async function init() {
  // Wait a bit for wails bindings to be available
  await waitForWails();

  // Load .env defaults
  try {
    const defaults = await callGo('GetEnvDefaults');
    if (defaults.site) document.getElementById('site-url').value = defaults.site;
    if (defaults.cookies) document.getElementById('cookies').value = defaults.cookies;
  } catch (e) {
    // ignore
  }

  // Set default mountpoint
  document.getElementById('mountpoint').value = '~/downleaf';

  // Check if already logged in
  try {
    const status = await callGo('GetLoginStatus');
    if (status.loggedIn) {
      onLoggedIn(status);
    }
  } catch (e) {
    // ignore
  }

  // Listen for log events
  if (window.runtime) {
    window.runtime.EventsOn('log', (line) => {
      appendLog(line);
    });
    window.runtime.EventsOn('mountStatusChanged', () => {
      refreshMountStatus();
    });
  }

  // Load existing logs
  try {
    const logs = await callGo('GetLogs');
    if (logs && logs.length > 0) {
      const el = document.getElementById('log-output');
      el.textContent = logs.join('\n') + '\n';
      el.scrollTop = el.scrollHeight;
    }
  } catch (e) {
    // ignore
  }
}

// Helper: wait for Wails bindings
function waitForWails() {
  return new Promise((resolve) => {
    const check = () => {
      if (window.go && window.go.gui && window.go.gui.App) {
        resolve();
      } else {
        setTimeout(check, 50);
      }
    };
    check();
  });
}

// Helper: call a Go method
async function callGo(method, ...args) {
  return window.go.gui.App[method](...args);
}

// ---- Login ----

async function doLogin() {
  const siteURL = document.getElementById('site-url').value.trim();
  const cookies = document.getElementById('cookies').value.trim();
  const errEl = document.getElementById('login-error');
  const btn = document.getElementById('login-btn');

  errEl.textContent = '';
  if (!siteURL || !cookies) {
    errEl.textContent = 'Please enter both Site URL and Cookie.';
    return;
  }

  btn.disabled = true;
  btn.textContent = 'Connecting...';

  try {
    const status = await callGo('Login', siteURL, cookies);
    onLoggedIn(status);
  } catch (e) {
    errEl.textContent = e.message || String(e);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Connect';
  }
}

// Make functions available globally for onclick handlers
window.doLogin = doLogin;

function onLoggedIn(status) {
  // Update header
  document.getElementById('header-status').innerHTML =
    `<span class="status-dot online"></span> ${status.email}`;

  // Hide login, show main
  document.getElementById('login-panel').style.display = 'none';
  document.getElementById('main-panel').style.display = 'flex';

  // Load projects
  refreshProjects();
  refreshMountStatus();
}

// ---- Projects ----

async function refreshProjects() {
  const listEl = document.getElementById('project-list');
  const selectEl = document.getElementById('project-select');

  listEl.innerHTML = '<div style="padding:10px;color:var(--text-secondary)">Loading...</div>';

  try {
    const projects = await callGo('ListProjects');

    // Update project list
    if (!projects || projects.length === 0) {
      listEl.innerHTML = '<div style="padding:10px;color:var(--text-secondary)">No projects found.</div>';
    } else {
      listEl.innerHTML = projects.map(p => `
        <div class="project-item">
          <div>
            <div class="project-name">${esc(p.name)}</div>
            <div class="project-id">${esc(p._id)}</div>
          </div>
          <div class="project-meta">${esc(p.accessLevel || '')}</div>
        </div>
      `).join('');
    }

    // Update select dropdown
    const currentVal = selectEl.value;
    selectEl.innerHTML = '<option value="">All Projects</option>';
    if (projects) {
      projects.forEach(p => {
        const opt = document.createElement('option');
        opt.value = p.name;
        opt.textContent = p.name;
        selectEl.appendChild(opt);
      });
    }
    selectEl.value = currentVal;
  } catch (e) {
    listEl.innerHTML = `<div class="error-msg">${esc(e.message || String(e))}</div>`;
  }
}

window.refreshProjects = refreshProjects;

// ---- Mount ----

async function doMount() {
  const projectName = document.getElementById('project-select').value;
  const mountpoint = document.getElementById('mountpoint').value.trim();
  const batchMode = document.getElementById('batch-mode').checked;
  const btn = document.getElementById('mount-btn');
  const statusEl = document.getElementById('mount-status');

  btn.disabled = true;
  btn.textContent = 'Mounting...';
  statusEl.textContent = '';

  try {
    await callGo('Mount', projectName, mountpoint, batchMode);
    refreshMountStatus();
  } catch (e) {
    statusEl.textContent = e.message || String(e);
    statusEl.className = 'status-bar';
  } finally {
    btn.disabled = false;
    btn.textContent = 'Mount';
  }
}

async function doUnmount() {
  const btn = document.getElementById('unmount-btn');
  btn.disabled = true;
  btn.textContent = 'Unmounting...';

  try {
    await callGo('Unmount');
    refreshMountStatus();
  } catch (e) {
    document.getElementById('mount-status').textContent = e.message || String(e);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Unmount';
  }
}

async function doSync() {
  const btn = document.getElementById('sync-btn');
  btn.disabled = true;
  btn.textContent = 'Syncing...';

  try {
    const msg = await callGo('Sync');
    document.getElementById('mount-status').textContent = msg;
  } catch (e) {
    document.getElementById('mount-status').textContent = e.message || String(e);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Sync';
  }
}

async function doOpen() {
  try {
    await callGo('OpenMountpoint');
  } catch (e) {
    // ignore
  }
}

window.doMount = doMount;
window.doUnmount = doUnmount;
window.doSync = doSync;
window.doOpen = doOpen;

async function refreshMountStatus() {
  try {
    const s = await callGo('GetMountStatus');
    const mountBtn = document.getElementById('mount-btn');
    const unmountBtn = document.getElementById('unmount-btn');
    const syncBtn = document.getElementById('sync-btn');
    const openBtn = document.getElementById('open-btn');
    const statusEl = document.getElementById('mount-status');

    if (s.mounted) {
      mountBtn.style.display = 'none';
      unmountBtn.style.display = '';
      syncBtn.style.display = s.batchMode ? '' : 'none';
      openBtn.style.display = '';
      statusEl.textContent = `Mounted at ${s.mountpoint}` + (s.project ? ` (${s.project})` : ' (all projects)');
      statusEl.className = 'status-bar mounted';

      // Disable mount config while mounted
      document.getElementById('project-select').disabled = true;
      document.getElementById('mountpoint').disabled = true;
      document.getElementById('batch-mode').disabled = true;
    } else {
      mountBtn.style.display = '';
      unmountBtn.style.display = 'none';
      syncBtn.style.display = 'none';
      openBtn.style.display = 'none';
      statusEl.textContent = '';
      statusEl.className = 'status-bar';

      document.getElementById('project-select').disabled = false;
      document.getElementById('mountpoint').disabled = false;
      document.getElementById('batch-mode').disabled = false;
    }
  } catch (e) {
    // ignore
  }
}

// ---- Logs ----

function appendLog(line) {
  const el = document.getElementById('log-output');
  el.textContent += line + '\n';
  // Auto-scroll to bottom
  el.scrollTop = el.scrollHeight;
}

function clearLogs() {
  document.getElementById('log-output').textContent = '';
}

window.clearLogs = clearLogs;

// ---- Util ----

function esc(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
