// Reusable shell components: product cards, modal, auth, palette, and workspace picker.
import React, { lazy, Suspense, useEffect, useRef, useState } from 'react';

const MarkdownRenderer = lazy(() => import('./markdown.jsx'));

export function Markdown({ value, className = '' }) {
  // 首次渲染 Markdown 时再加载解析器；加载期间保留可读文本，避免消息区域空白或跳动。
  const fallback = <div className={className} aria-busy="true">{value || ''}</div>;
  return <Suspense fallback={fallback}><MarkdownRenderer value={value} className={className} /></Suspense>;
}

export function TextCard({ title, hint, badge, badgeClass = '', active, children }) {
  return <div className={'product-card ' + (active ? 'active' : '')}>
    <div className="product-card-head"><div><b>{title}</b>{hint ? <div className="hint">{hint}</div> : null}</div>{badge ? <span className={'badge ' + badgeClass}>{badge}</span> : null}</div>
    {children}
  </div>;
}

export function QuickPalette({ open, actions, onClose }) {
  const [query, setQuery] = useState('');
  const inputRef = useRef(null);
  useEffect(() => {
    if (!open) {
      setQuery('');
      return;
    }
    const id = requestAnimationFrame(() => inputRef.current?.focus());
    return () => cancelAnimationFrame(id);
  }, [open]);
  if (!open) return null;
  const q = query.trim().toLowerCase();
  const filtered = actions.filter(action => !q || [action.title, action.hint, action.id].some(v => String(v || '').toLowerCase().includes(q)));
  function runAction(action) {
    if (!action || action.disabled) return;
    onClose();
    action.run?.();
  }
  return <div className="quick-palette-backdrop show" onClick={onClose}>
    <div className="quick-palette" role="dialog" aria-modal="true" aria-label="快捷指令" onClick={e => e.stopPropagation()}>
      <div className="quick-palette-head">
        <input ref={inputRef} value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => {
          if (e.key === 'Escape') onClose();
          if (e.key === 'Enter') runAction(filtered.find(a => !a.disabled));
        }} placeholder="搜索快捷指令，例如：模型、导出、工作空间" />
        <button className="secondary small quick-palette-close" onClick={onClose} aria-label="关闭快捷指令">×</button>
      </div>
      <div className="quick-palette-list">
        {filtered.length ? filtered.map(action => <button key={action.id} type="button" className="quick-palette-item" disabled={!!action.disabled} onClick={() => runAction(action)}>
          <span><b>{action.title}</b><small>{action.hint}</small></span>
          <em>{action.disabled ? '不可用' : '执行'}</em>
        </button>) : <div className="empty compact">没有匹配的快捷指令。</div>}
      </div>
      <div className="quick-palette-foot">快捷键：⌘/Ctrl K 打开，/ 聚焦输入框，Esc 关闭弹层。</div>
    </div>
  </div>;
}

export function WorkspacePicker({ open, workspaceSummaries, busy, activeName, onClose, onSelect }) {
  if (!open) return null;
  return <div className="workspace-picker-backdrop show" onClick={onClose}>
    <div className="workspace-picker-sheet" role="dialog" aria-modal="true" aria-label="选择工作空间" onClick={e => e.stopPropagation()}>
      <div className="workspace-picker-head">
        <div><b>选择工作空间</b><div className="hint">切换后会加载对应会话和模型。</div></div>
        <button className="secondary small" type="button" onClick={onClose}>关闭</button>
      </div>
      <div className="workspace-picker-list">
        {workspaceSummaries.length ? workspaceSummaries.map(item => <button
          key={item.name}
          type="button"
          disabled={busy}
          className={'workspace-picker-item ' + (item.name === activeName ? 'active' : '')}
          onClick={() => onSelect(item.name)}>
          <span className="workspace-picker-item-main"><b>{item.name}</b><span>{item.count} 条会话</span></span>
          <span className="workspace-picker-check">{item.name === activeName ? '✓' : ''}</span>
        </button>) : <div className="empty compact">暂无工作空间。</div>}
      </div>
    </div>
  </div>;
}

export function LoginPage({ api, error, refreshAfterLogin, setAuthPage }) {
  const [username, setUsername] = useState('');
  const [credential, setCredential] = useState('');
  const [loginError, setLoginError] = useState('');
  const message = error ? (error.status === 401 ? '登录已过期，请重新登录。' : error.message) : '请输入 ChatDock 账号和密码。';
  const canSubmit = username.trim() && credential;
  async function submit(event) {
    event.preventDefault();
    setLoginError('');
    try {
      const data = await api('/api/auth/login', {method:'POST', body: JSON.stringify({username: username.trim(), credential})});
      if (data.token) localStorage.setItem('chatdock.authToken', data.token);
      setAuthPage(null);
      await refreshAfterLogin();
    } catch (e) { setLoginError('登录失败：' + e.message); }
  }
  return <div id="authPage" className="auth-page">
    <div className="auth-ambient auth-ambient-one" aria-hidden="true" />
    <div className="auth-ambient auth-ambient-two" aria-hidden="true" />
    <div className="auth-shell">
      <section className="auth-intro" aria-label="ChatDock 简介">
        <div className="auth-logo">✦</div>
        <div className="auth-eyebrow">Local-first AI console</div>
        <h1>ChatDock</h1>
        <p>把会话、模型供应商、工具和定时任务收进一个轻量工作台。</p>
        <div className="auth-feature-grid">
          <span>多工作空间</span>
          <span>模型路由</span>
          <span>MCP 工具</span>
          <span>定时任务</span>
        </div>
      </section>
      <form className="login-card" onSubmit={submit}>
        <div className="login-card-head">
          <div>
            <div className="login-brand">ChatDock</div>
            <b>欢迎回来</b>
          </div>
          <span>私有访问</span>
        </div>
        <div className="hint">{message}</div>
        <label>账号</label><input autoComplete="username" placeholder="输入账号" value={username} onChange={e => setUsername(e.target.value)} autoFocus />
        <label>密码</label><input type="password" autoComplete="current-password" placeholder="输入密码" value={credential} onChange={e => setCredential(e.target.value)} />
        <div className="task-error" role="alert">{loginError}</div>
        <button type="submit" className="login-submit" disabled={!canSubmit}>登录并进入</button>
        <div className="login-footnote">访问凭证只保存在当前浏览器本地。</div>
      </form>
    </div>
  </div>;
}

export function DialogHost({ dialog, closeDialog }) {
  const [values, setValues] = useState({});
  useEffect(() => {
    const next = {};
    (dialog?.fields || []).forEach(f => { next[f.name] = f.value ?? ''; });
    setValues(next);
  }, [dialog]);
  if (!dialog) return null;
  const visibleFields = (dialog.fields || []).filter(field => {
    if (!field.showWhen) return true;
    return Object.entries(field.showWhen).every(([key, expected]) => {
      const current = values[key];
      return Array.isArray(expected) ? expected.includes(current) : current === expected;
    });
  });
  function submit(event) {
    event.preventDefault();
    if (dialog.type === 'confirm') closeDialog(true);
    else closeDialog(values);
  }
  return <div className="app-modal-backdrop show" onClick={e => { if (e.target === e.currentTarget) closeDialog(null); }}>
    <div className={'app-modal-card ' + (dialog.variant || '')} role="dialog" aria-modal="true"><form className="app-modal-form" onSubmit={submit}>
      <div className="app-modal-title">{dialog.title || '确认'}</div>
      {dialog.message ? <div className="app-modal-message">{dialog.message}</div> : null}
      {dialog.toolEventDetail ? <ToolEventDetail detail={dialog.toolEventDetail} /> : null}
      <div className="app-modal-fields">{visibleFields.map(field => field.type === 'hidden' ? renderDialogField(field, values[field.name] ?? '', value => setValues(v => ({...v, [field.name]: value})), values, setValues) : <label key={field.name} className={'app-modal-field ' + (field.type === 'provider_keys' ? 'provider-keys-field' : '')}><span>{field.label || field.name}</span>{renderDialogField(field, values[field.name] ?? '', value => setValues(v => ({...v, [field.name]: value})), values, setValues)}{field.hint ? <div className="app-modal-field-hint">{field.hint}</div> : null}</label>)}</div>
      <div className="app-modal-actions">{dialog.hideCancel ? null : <button type="button" className="secondary app-modal-cancel" onClick={() => closeDialog(null)}>{dialog.cancelText || '取消'}</button>}<button type="submit" className={dialog.danger ? 'danger' : ''}>{dialog.confirmText || '确定'}</button></div>
    </form></div>
  </div>;
}


function ToolEventDetail({ detail }) {
  if (!detail) return null;
  const metrics = Array.isArray(detail.metrics) ? detail.metrics.filter(item => item?.value != null && item.value !== '') : [];
  const rows = Array.isArray(detail.rows) ? detail.rows.filter(item => item?.value != null && item.value !== '') : [];
  const sections = Array.isArray(detail.sections) ? detail.sections : [];
  const kicker = detail.primary?.label || detail.event || 'tool event';
  return <div className="tool-event-detail">
    <div className="tool-event-summary">
      <div className="tool-event-summary-main">
        <div className="tool-event-kicker">{kicker}</div>
        <div className="tool-event-heading">{detail.heading || detail.primary?.name || '工具事件'}</div>
        {detail.subheading ? <div className="tool-event-subheading">{detail.subheading}</div> : null}
      </div>
      {detail.status ? <span className={'tool-event-status ' + (detail.statusTone || '')}>{detail.status}</span> : null}
    </div>
    {metrics.length ? <div className="tool-event-metrics">{metrics.map(item => <div key={item.label} className="tool-event-metric"><strong>{item.value}</strong><span>{item.label}</span></div>)}</div> : null}
    {rows.length ? <div className="tool-event-rows">{rows.map(item => <div key={item.label} className="tool-event-row"><span>{item.label}</span><b>{String(item.value)}</b></div>)}</div> : null}
    <div className="tool-event-sections">{sections.map(section => <ToolEventSection key={section.title} section={section} />)}</div>
  </div>;
}

function ToolEventSection({ section }) {
  const className = 'tool-event-section ' + (section.tone || '') + (section.muted ? ' muted' : '');
  const body = section.display === 'tools'
    ? <div className="tool-event-tool-list">{(Array.isArray(section.value) ? section.value : []).map(name => <span key={String(name)}>{String(name)}</span>)}</div>
    : <pre>{formatDialogValue(section.value, section.emptyText)}</pre>;
  if (section.collapsed) {
    return <details className={className}>
      <summary className="tool-event-section-title">{section.title}</summary>
      {body}
    </details>;
  }
  return <section className={className}>
    <div className="tool-event-section-title">{section.title}</div>
    {body}
  </section>;
}

function formatDialogValue(value, emptyText = '无') {
  if (value == null || value === '') return emptyText;
  if (typeof value === 'string') return value;
  try { return JSON.stringify(value, null, 2); }
  catch { return String(value); }
}


function ProviderKeysEditor({ value, setValue, values, setValues }) {
  const rows = Array.isArray(value) && value.length ? value : [{id: 'main', name: '主 key', api_key: '', enabled: true, priority: 1}];
  const selectedID = String(values?.selected_key_id || rows[0]?.id || '').trim();
  const updateRow = (index, patch) => setValue(rows.map((row, i) => i === index ? {...row, ...patch} : row));
  const setSelected = (id) => setValues?.(current => ({...current, selected_key_id: id}));
  const addRow = () => {
    const index = rows.length + 1;
    const id = 'key-' + index;
    const next = [...rows, {id, name: '备用 key ' + (index - 1), api_key: '', enabled: true, priority: index, saved: false}];
    setValue(next);
    if (!selectedID) setSelected(id);
  };
  const removeRow = (index) => {
    const removedID = rows[index]?.id;
    const next = rows.filter((_, i) => i !== index);
    const fallback = next.length ? next : [{id: 'main', name: '主 key', api_key: '', enabled: true, priority: 1}];
    setValue(fallback);
    if (selectedID === removedID) setSelected((fallback[0] || {}).id || 'main');
  };
  return <div className="provider-keys-editor simplified">
    <div className="provider-keys-list">{rows.map((row, index) => <div className="provider-key-row" key={row.id || index}>
      <label className="provider-key-current" title="设为当前 Key"><input type="radio" name="provider-current-key" checked={(selectedID || rows[0]?.id) === row.id} onChange={() => setSelected(row.id)} /><span>当前</span></label>
      <input value={row.name || ''} placeholder={index === 0 ? '主 key' : '备用 key'} onChange={e => updateRow(index, {name: e.target.value})} />
      <input type="text" className="provider-key-secret" value={row.api_key || ''} placeholder="粘贴 Key；已保存 Key 只隐藏中间" onChange={e => updateRow(index, {api_key: e.target.value})} />
      <label className="provider-key-enabled"><input type="checkbox" checked={row.enabled !== false} onChange={e => updateRow(index, {enabled: e.target.checked})} /><span>{row.enabled === false ? '停用' : '启用'}</span></label>
      <button type="button" className="secondary small" onClick={() => removeRow(index)} disabled={rows.length <= 1}>删除</button>
    </div>)}</div>
    <button type="button" className="secondary small provider-key-add" onClick={addRow}>+ 添加备用 Key</button>
  </div>;
}

function renderDialogField(field, value, setValue, values = {}, setValues = null) {
  if (field.type === 'hidden') return <input key={field.name} type="hidden" value={value || ''} readOnly />;
  if (field.type === 'provider_keys') return <ProviderKeysEditor value={value} setValue={setValue} values={values} setValues={setValues} />;
  if (field.type === 'textarea') return <textarea rows={field.rows || 5} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
  if (field.type === 'select') return <select value={value} required={!!field.required} onChange={e => { const next = e.target.value; setValue(next); if (field.fillByValue && setValues) { const fill = field.fillByValue[next] || {}; setValues(current => ({...current, ...Object.fromEntries(Object.entries(fill).filter(([, v]) => v))})); } }}>{(field.options || []).map(opt => typeof opt === 'string' ? <option key={opt} value={opt}>{opt}</option> : <option key={opt.value} value={opt.value}>{opt.label}</option>)}</select>;
  return <input type={field.type || 'text'} min={field.min} max={field.max} step={field.step} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
}
