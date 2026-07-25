// Reusable shell components: product cards, modal, auth, and palette.
import React, { lazy, Suspense, useEffect, useRef, useState } from 'react';
import {
  LoaderCircle,
  Orbit,
  Search,
  X,
} from './icons.js';
import { ScheduleBuilder } from './scheduleBuilder.jsx';

const MarkdownRenderer = lazy(() => import('./markdown.jsx'));

export function Markdown({ value, className = '' }) {
  // 首次渲染 Markdown 时再加载解析器；加载期间保留可读文本，避免消息区域空白或跳动。
  const fallback = <div className={className} aria-busy="true">{value || ''}</div>;
  return <Suspense fallback={fallback}><MarkdownRenderer value={value} className={className} /></Suspense>;
}

export function PageLoadingState({ title = '正在加载', detail = '正在准备页面内容。', fullscreen = false }) {
  return <div className={'route-loading-state ' + (fullscreen ? 'fullscreen' : 'content')} role="status" aria-live="polite" aria-busy="true">
    <div className="route-loading-card">
      <span className="route-loading-icon" aria-hidden="true"><LoaderCircle size={20} /></span>
      <div className="route-loading-copy">
        <span className="route-loading-eyebrow">CHATDOCK</span>
        <strong>{title}</strong>
        <span>{detail}</span>
      </div>
      <span className="route-loading-progress" aria-hidden="true"><span /></span>
    </div>
  </div>;
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
        <Search className="quick-palette-search-icon" size={17} aria-hidden="true" /><input ref={inputRef} value={query} onChange={e => setQuery(e.target.value)} onKeyDown={e => {
          if (e.key === 'Escape') onClose();
          if (e.key === 'Enter') runAction(filtered.find(a => !a.disabled));
        }} placeholder="搜索快捷指令，例如：模型、导出、项目" />
        <button className="secondary small quick-palette-close icon-button" onClick={onClose} aria-label="关闭快捷指令"><X size={16} aria-hidden="true" /></button>
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
    <div className="auth-grid" aria-hidden="true" />
    <div className="auth-shell">
      <section className="auth-intro" aria-label="ChatDock 简介">
        <div className="auth-brand-lockup"><span className="auth-logo"><Orbit size={22} /></span><span>CHATDOCK / PRIVATE</span></div>
        <div className="auth-eyebrow">Local-first AI console</div>
        <h1>工作流，<br /><span>不止对话。</span></h1>
        <p>把模型、工具、项目与自动化放进同一条可追踪的执行链。</p>
        <div className="auth-feature-grid">
          <span>项目上下文</span>
          <span>模型路由</span>
          <span>工具执行</span>
          <span>任务自动化</span>
        </div>
      </section>
      <form className="login-card" onSubmit={submit}>
        <div className="login-card-head">
          <div>
            <div className="login-brand">Private access</div>
            <b>继续到你的工作台</b>
          </div>
          <span>Secure</span>
        </div>
        <div className="hint">{message}</div>
        <label><span>账号</span><div className="login-input-wrap"><input autoComplete="username" placeholder="输入账号" value={username} onChange={e => setUsername(e.target.value)} autoFocus /></div></label>
        <label><span>密码</span><div className="login-input-wrap"><input type="password" autoComplete="current-password" placeholder="输入密码" value={credential} onChange={e => setCredential(e.target.value)} /></div></label>
        <div className="task-error" role="alert">{loginError}</div>
        <button type="submit" className="login-submit" disabled={!canSubmit}>登录并进入</button>
        <div className="login-footnote">凭证只保存在当前浏览器本地。</div>
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
      <div className="app-modal-fields">{visibleFields.map(field => {
        const value = values[field.name] ?? '';
        const setValue = next => setValues(current => ({...current, [field.name]: next}));
        const control = renderDialogField(field, value, setValue, values, setValues);
        if (field.type === 'hidden') return control;
        if (field.type === 'schedule_builder') return <div key={field.name} className="app-modal-field schedule-builder-field"><span>{field.label || field.name}</span>{control}{field.hint ? <div className="app-modal-field-hint">{field.hint}</div> : null}</div>;
        return <label key={field.name} className={'app-modal-field ' + (field.type === 'provider_keys' ? 'provider-keys-field' : '')}><span>{field.label || field.name}</span>{control}{field.hint ? <div className="app-modal-field-hint">{field.hint}</div> : null}</label>;
      })}</div>
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
  if (field.type === 'schedule_builder') return <ScheduleBuilder value={value} onChange={setValue} />;
  if (field.type === 'textarea') return <textarea rows={field.rows || 5} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
  if (field.type === 'select') return <select value={value} required={!!field.required} onChange={e => { const next = e.target.value; setValue(next); if (field.fillByValue && setValues) { const fill = field.fillByValue[next] || {}; setValues(current => ({...current, ...Object.fromEntries(Object.entries(fill).filter(([, v]) => v))})); } }}>{(field.options || []).map(opt => typeof opt === 'string' ? <option key={opt} value={opt}>{opt}</option> : <option key={opt.value} value={opt.value}>{opt.label}</option>)}</select>;
  return <input type={field.type || 'text'} min={field.min} max={field.max} step={field.step} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
}
