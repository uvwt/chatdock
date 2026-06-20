// ChatDock module auth：API 包装、登录页和启动入口。
function authHeaders(extra={}) {
  const token = localStorage.getItem('chatdock.authToken') || '';
  return token ? {'Authorization':'Bearer ' + token, ...extra} : extra;
}

function authURL(path) {
  const token = localStorage.getItem('chatdock.authToken') || '';
  if (!token) return path;
  const sep = path.includes('?') ? '&' : '?';
  return path + sep + 'token=' + encodeURIComponent(token);
}

async function api(path, opt={}) {
  const res = await fetch(path, {...opt, headers: authHeaders({'Content-Type':'application/json', ...(opt.headers || {})})});
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    err.path = path;
    if (res.status === 401 && path !== '/api/auth/login') showLoginPage(err);
    throw err;
  }
  return data;
}

function loginFormHTML(error) {
  const message = error ? (error.status === 401 ? '登录已过期，请重新登录。' : error.message) : '请输入 ChatDock 账号和密码。';
  return '<form class="login-card" data-action="login-submit">' +
    '<div class="login-brand">ChatDock</div>' +
    '<b>登录 ChatDock</b>' +
    '<div class="hint">' + escapeHtml(message) + '</div>' +
    '<label>账号</label><input id="login_username" autocomplete="username" placeholder="账号" />' +
    '<label>密码</label><input id="login_credential" type="password" autocomplete="current-password" placeholder="密码" />' +
    '<div id="loginError" class="task-error"></div>' +
    '<button type="submit" class="login-submit">登录</button>' +
  '</form>';
}

function panelErrorHTML(error, retryAction) {
  if (error && error.status === 401) return '<div class="empty compact">登录已过期，请重新登录。</div>';
  return '<div class="empty compact error-state"><b>加载失败</b><div class="hint">' + escapeHtml(error.message || '请稍后重试') + '</div><div class="settings-actions">' +
    '<button class="secondary small" data-action="' + dataAttr(retryAction) + '">重试</button></div></div>';
}

function renderPanelError(target, error, retryAction) {
  if (target) target.innerHTML = panelErrorHTML(error, retryAction);
}

function showLoginForm() {
  showLoginPage();
}

function showLoginPage(error) {
  document.body.classList.add('auth-page-visible');
  let page = document.getElementById('authPage');
  if (!page) {
    page = document.createElement('div');
    page.id = 'authPage';
    page.className = 'auth-page';
    document.body.appendChild(page);
  }
  page.innerHTML = '<div class="auth-shell">' + loginFormHTML(error) + '</div>';
  const username = document.getElementById('login_username');
  if (username) username.focus();
}

function hideLoginPage() {
  document.body.classList.remove('auth-page-visible');
  const page = document.getElementById('authPage');
  if (page) page.remove();
}

async function submitLogin(event) {
  event.preventDefault();
  const username = ((document.getElementById('login_username') || {}).value || '').trim();
  const credential = ((document.getElementById('login_credential') || {}).value || '').trim();
  const errorBox = document.getElementById('loginError');
  if (errorBox) errorBox.textContent = '';
  try {
    const data = await api('/api/auth/login', {method:'POST', body: JSON.stringify({username, credential})});
    if (data.token) localStorage.setItem('chatdock.authToken', data.token);
    hideLoginPage();
    await refreshAfterLogin();
  } catch (e) {
    if (errorBox) errorBox.textContent = '登录失败：' + e.message;
  }
}

async function refreshAfterLogin() {
  await Promise.allSettled([refreshProductState(), loadConfig(), loadMCPConfig(), loadSkills(), loadScheduledTasks(), loadSessions()]);
}

async function ensureAuthenticated() {
  try {
    const status = await api('/api/auth/status');
    if (status.enabled && status.login_enabled && !localStorage.getItem('chatdock.authToken')) {
      showLoginPage();
      return false;
    }
    hideLoginPage();
    return true;
  } catch (e) {
    if (e.status === 401) {
      showLoginPage(e);
      return false;
    }
    throw e;
  }
}

async function startApp() {
  initTheme();
  initSidebar();
  initSettingsRoute();
  initDelegatedActions();
  try {
    const ok = await ensureAuthenticated();
    if (!ok) return;
    await refreshAfterLogin();
  } catch (e) {
    showLoginPage(e);
  }
}
