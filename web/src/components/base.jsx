// Reusable shell components: product cards, modal, auth, and palette.
import React, { lazy, Suspense, useEffect, useRef, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Bot,
  LoaderCircle,
  Orbit,
  Sun,
  X,
} from './icons.js';
import { ScheduleBuilder } from './scheduleBuilder.jsx';
import { loadMarkdownRenderer, markdownRendererIfReady } from '../lib/markdownLoader.js';

const MarkdownRenderer = lazy(loadMarkdownRenderer);

export function Markdown({ value, className = '' }) {
  // 分包已就绪时直接同步渲染，跳过 React.lazy 首帧必然 suspend 的那一次裸文本。
  const ready = markdownRendererIfReady();
  if (ready) {
    const Renderer = ready.default;
    return <Renderer value={value} className={className} />;
  }
  // 尚未就绪时保留可读文本，避免消息区域空白或跳动。
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

const quickActionIcons = {
  continue: ArrowUp,
  'provider-system-prompt': Bot,
  'export-session': ArrowDown,
  theme: Sun,
};

export function QuickPalette({ open, actions, onClose }) {
  const paletteRef = useRef(null);
  const firstActionRef = useRef(null);
  const returnFocusRef = useRef(null);
  const restoreFocusRef = useRef(true);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    if (!open) return undefined;
    returnFocusRef.current = document.activeElement;
    restoreFocusRef.current = true;
    const frame = requestAnimationFrame(() => firstActionRef.current?.focus());
    const handlePaletteKeyDown = event => {
      if (event.key === 'Tab') {
        const focusable = Array.from(paletteRef.current?.querySelectorAll('button:not(:disabled)') || []);
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        restoreFocusRef.current = true;
        onCloseRef.current();
      }
    };
    document.addEventListener('keydown', handlePaletteKeyDown);
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener('keydown', handlePaletteKeyDown);
      if (!restoreFocusRef.current) return;
      const target = returnFocusRef.current;
      requestAnimationFrame(() => {
        if (target && document.contains(target)) target.focus?.();
      });
    };
  }, [open]);

  if (!open) return null;

  const firstEnabledActionID = actions.find(action => !action.disabled)?.id;
  const groups = actions.reduce((result, action) => {
    const label = action.group || '其他';
    let group = result.find(item => item.label === label);
    if (!group) {
      group = {label, actions: []};
      result.push(group);
    }
    group.actions.push(action);
    return result;
  }, []);

  function closePalette() {
    restoreFocusRef.current = true;
    onClose();
  }

  function runAction(action) {
    if (!action || action.disabled) return;
    restoreFocusRef.current = false;
    onClose();
    action.run?.();
  }

  return <div className="quick-palette-backdrop show" onClick={closePalette}>
    <div ref={paletteRef} className="quick-palette" role="dialog" aria-modal="true" aria-labelledby="quickPaletteTitle" aria-describedby="quickPaletteDescription" onClick={event => event.stopPropagation()}>
      <div className="quick-palette-head">
        <div className="quick-palette-heading">
          <span>QUICK ACTIONS</span>
          <strong id="quickPaletteTitle">快捷操作</strong>
          <p id="quickPaletteDescription">当前会话与界面的常用入口</p>
        </div>
        <button type="button" className="secondary small quick-palette-close icon-button" onClick={closePalette} aria-label="关闭快捷操作"><X size={17} aria-hidden="true" /></button>
      </div>
      <div className="quick-palette-list">
        {groups.map((group, groupIndex) => <section className="quick-palette-group" key={group.label} aria-labelledby={'quickPaletteGroup' + groupIndex}>
          <div className="quick-palette-group-head">
            <span id={'quickPaletteGroup' + groupIndex}>{group.label}</span>
            <small>{group.actions.filter(action => !action.disabled).length} / {group.actions.length} 可用</small>
          </div>
          <div className="quick-palette-group-actions">
            {group.actions.map(action => {
              const ActionIcon = quickActionIcons[action.id] || Orbit;
              return <button
                key={action.id}
                ref={action.id === firstEnabledActionID ? firstActionRef : null}
                type="button"
                className="quick-palette-item"
                disabled={!!action.disabled}
                onClick={() => runAction(action)}
              >
                <span className="quick-palette-item-icon" aria-hidden="true"><ActionIcon size={18} strokeWidth={1.8} /></span>
                <span className="quick-palette-item-copy"><b>{action.title}</b><small>{action.hint}</small></span>
                <em className={action.disabled ? 'disabled' : ''} aria-hidden="true">{action.disabled ? '不可用' : '→'}</em>
              </button>;
            })}
          </div>
        </section>)}
      </div>
      <div className="quick-palette-foot">
        <span>键盘快捷键</span>
        <div className="quick-palette-shortcuts"><span><kbd>⌘ / Ctrl K</kbd> 打开</span><span><kbd>Esc</kbd> 关闭</span></div>
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
        if (field.type === 'readonly_text') return <section key={field.name} className="app-modal-field readonly-text-field"><span>{field.label || field.name}</span>{control}{field.hint ? <div className="app-modal-field-hint">{field.hint}</div> : null}</section>;
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


export function ProviderKeysEditor({ value, setValue, values, setValues }) {
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
  if (field.type === 'readonly_text') return <div className="app-modal-readonly-text" role="document" tabIndex={0}><pre>{value || field.emptyText || '（空）'}</pre></div>;
  if (field.type === 'textarea') return <textarea rows={field.rows || 5} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
  if (field.type === 'select') return <select value={value} required={!!field.required} onChange={e => { const next = e.target.value; setValue(next); if (field.fillByValue && setValues) { const fill = field.fillByValue[next] || {}; setValues(current => ({...current, ...Object.fromEntries(Object.entries(fill).filter(([, v]) => v))})); } }}>{(field.options || []).map(opt => typeof opt === 'string' ? <option key={opt} value={opt}>{opt}</option> : <option key={opt.value} value={opt.value}>{opt.label}</option>)}</select>;
  return <input type={field.type || 'text'} min={field.min} max={field.max} step={field.step} value={value} placeholder={field.placeholder || ''} required={!!field.required} onChange={e => setValue(e.target.value)} />;
}
