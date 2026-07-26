import React, { useEffect, useMemo, useState } from 'react';
import { ArrowLeft } from './icons.js';
import { ProviderKeysEditor } from './base.jsx';
import { ScheduleBuilder } from './scheduleBuilder.jsx';
import { cronScheduleFormValue, defaultRunAtValue, fmtTime } from '../lib/appUtils.js';
import { providerKeyRows, uniqueModelNames } from '../lib/modelProviderForm.js';

export function SettingsEditorPage({ title, description, onBack, children, actions, eyebrow = '配置' }) {
  useEffect(() => {
    const handleEscape = event => {
      if (event.key !== 'Escape' || document.querySelector('.app-modal-backdrop.show')) return;
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation?.();
      onBack?.();
    };
    window.addEventListener('keydown', handleEscape, true);
    return () => window.removeEventListener('keydown', handleEscape, true);
  }, [onBack]);
  return <section className="settings-editor-page">
    <header className="settings-editor-header">
      <button type="button" className="secondary icon-button settings-editor-back" onClick={onBack} aria-label="返回列表" title="返回列表"><ArrowLeft size={17} aria-hidden="true" /></button>
      <div className="settings-editor-heading"><span>{eyebrow}</span><h2>{title}</h2>{description ? <p>{description}</p> : null}</div>
    </header>
    <div className="settings-editor-body">{children}</div>
    {actions ? <footer className="settings-editor-actions">{actions}</footer> : null}
  </section>;
}

function useSaving() {
  const [saving, setSaving] = useState(false);
  const run = async action => {
    if (saving) return false;
    setSaving(true);
    try { return await action(); }
    finally { setSaving(false); }
  };
  return {saving, run};
}

export function ProviderEditor({ provider, onBack, onDelete, onSave, onTest }) {
  const initial = useMemo(() => {
    const keys = providerKeyRows(provider);
    return {
      name: provider?.name || '',
      base_url: provider?.base_url || '',
      default_model: provider?.default_model || '',
      selected_key_id: provider?.selected_key_id || keys[0]?.id || 'main',
      key_strategy: provider?.key_strategy || 'auto',
      api_keys: keys,
      models: uniqueModelNames([...(provider?.models || []), provider?.default_model].filter(Boolean)).join('\n'),
      enabled: provider && provider.enabled === false ? 'false' : 'true',
    };
  }, [provider]);
  const [values, setValues] = useState(initial);
  const [error, setError] = useState('');
  const {saving, run} = useSaving();
  const isNew = !provider?.id;
  const update = (key, value) => setValues(current => ({...current, [key]: value}));
  const submit = async event => {
    event.preventDefault();
    setError('');
    if (!values.name.trim() || !values.base_url.trim() || !values.default_model.trim()) {
      setError('名称、Base URL 和默认模型不能为空。');
      return;
    }
    await run(async () => {
      try {
        const saved = await onSave(provider || null, values);
        if (saved !== false) onBack();
      } catch (saveError) {
        setError(saveError?.message || '保存供应商失败。');
      }
    });
  };
  return <SettingsEditorPage
    eyebrow="模型供应商"
    title={isNew ? '新增供应商' : '编辑 ' + (provider.name || provider.id)}
    description="管理连接地址、模型列表和 Key。保存后立即生效。"
    onBack={onBack}
    actions={<><div className="settings-editor-secondary-actions">{isNew ? null : <><button type="button" className="danger" onClick={() => onDelete(provider)} disabled={saving}>删除供应商</button><button type="button" className="secondary" onClick={() => onTest(provider)} disabled={saving}>测试已保存配置</button></>}</div><div className="settings-editor-primary-actions"><button type="button" className="secondary" onClick={onBack} disabled={saving}>取消</button><button type="submit" form="providerEditorForm" disabled={saving}>{saving ? '保存中…' : isNew ? '新增供应商' : '保存修改'}</button></div></>}
  >
    <form id="providerEditorForm" className="settings-editor-form" onSubmit={submit}>
      <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>基本信息</b><p>用于识别供应商并连接 OpenAI 兼容接口。</p></div></div>
        <div className="settings-editor-grid">
          <label>名称<input value={values.name} onChange={event => update('name', event.target.value)} placeholder="例如：火山 / OpenAI / Claude Proxy" required /></label>
          <label>状态<select value={values.enabled} onChange={event => update('enabled', event.target.value)}><option value="true">启用</option><option value="false">停用</option></select></label>
        </div>
        <label>Base URL<input value={values.base_url} onChange={event => update('base_url', event.target.value)} placeholder="https://example.com/v1" required /></label>
      </section>
      <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>模型</b><p>默认模型用于快速选择，可用模型会出现在聊天模型列表中。</p></div></div>
        <label>默认模型<input value={values.default_model} onChange={event => update('default_model', event.target.value)} placeholder="例如：gpt-5 / deepseek-v4" required /></label>
        <label>可用模型（每行一个）<textarea rows="7" value={values.models} onChange={event => update('models', event.target.value)} /></label>
      </section>
      <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>API Key</b><p>当前 Key 用单选按钮切换，已保存 Key 会以掩码显示。</p></div></div>
        <ProviderKeysEditor value={values.api_keys} setValue={value => update('api_keys', value)} values={values} setValues={setValues} />
      </section>
      {error ? <div className="backup-health warn" role="alert">{error}</div> : null}
    </form>
  </SettingsEditorPage>;
}

export function ProjectEditor({ project, promptPreview, onBack, onDelete, onOpenSessions, onPreview, onSave, onStartConversation }) {
  const [values, setValues] = useState({name: project?.name || '', prompt: project?.prompt || ''});
  const [error, setError] = useState('');
  const {saving, run} = useSaving();
  const isNew = !project?.id;
  const submit = async event => {
    event.preventDefault();
    if (!values.name.trim()) { setError('项目名称不能为空。'); return; }
    setError('');
    await run(async () => {
      try {
        const saved = await onSave(project || null, values);
        if (saved !== false) onBack();
      } catch (saveError) {
        setError(saveError?.message || '保存项目失败。');
      }
    });
  };
  return <SettingsEditorPage
    eyebrow="项目"
    title={isNew ? '新增项目' : '编辑 ' + project.name}
    description="项目名称、提示词和最终上下文在同一处维护。"
    onBack={onBack}
    actions={<><div className="settings-editor-secondary-actions">{isNew ? null : <><button type="button" className="danger" onClick={() => onDelete(project)} disabled={saving}>删除项目</button><button type="button" className="secondary" onClick={() => onStartConversation(project.id)} disabled={saving}>新建对话</button><button type="button" className="secondary" onClick={() => onOpenSessions(project.id)} disabled={saving}>查看会话</button></>}</div><div className="settings-editor-primary-actions"><button type="button" className="secondary" onClick={onBack} disabled={saving}>取消</button><button type="submit" form="projectEditorForm" disabled={saving}>{saving ? '保存中…' : isNew ? '创建项目' : '保存修改'}</button></div></>}
  >
    <form id="projectEditorForm" className="settings-editor-form" onSubmit={submit}>
      <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>项目设置</b><p>项目提示词会与全局系统提示词组合后发送给模型。</p></div></div>
        <label>项目名称<input value={values.name} onChange={event => setValues(current => ({...current, name: event.target.value}))} required /></label>
        <label>项目提示词<textarea rows="10" value={values.prompt} onChange={event => setValues(current => ({...current, prompt: event.target.value}))} placeholder="描述这个项目的目标、背景和长期约束" /></label>
      </section>
      {isNew ? null : <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>最终提示词预览</b><p>查看全局提示词与项目提示词组合后的实际内容。</p></div><button type="button" className="secondary small" onClick={() => onPreview(project.id)}>刷新预览</button></div>
        {promptPreview ? <pre className="settings-editor-preview">{promptPreview}</pre> : <div className="hint">点击“刷新预览”读取最终提示词。</div>}
      </section>}
      {error ? <div className="backup-health warn" role="alert">{error}</div> : null}
    </form>
  </SettingsEditorPage>;
}

function taskInitialValues(task) {
  return {
    title: task?.title || '',
    prompt: task?.prompt || '',
    schedule_type: task?.schedule_type || 'once',
    run_at: task?.run_at ? task.run_at.slice(0, 16) : defaultRunAtValue(),
    interval_minutes: task?.interval_minutes ? String(task.interval_minutes) : '60',
    cron_schedule: cronScheduleFormValue(task || {}, Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai'),
    context_mode: task?.context_mode || 'stateless',
    reschedule: false,
  };
}

export function ScheduledTaskEditor({ task, onBack, onDelete, onRun, onSave, onViewRuns }) {
  const [values, setValues] = useState(() => taskInitialValues(task));
  const [error, setError] = useState('');
  const {saving, run} = useSaving();
  const isNew = !task?.id;
  const update = (key, value) => setValues(current => ({...current, [key]: value}));
  const submit = async event => {
    event.preventDefault();
    if (!values.title.trim() || !values.prompt.trim()) { setError('任务标题和提示词不能为空。'); return; }
    setError('');
    await run(async () => {
      try {
        const saved = await onSave(task || null, values);
        if (saved !== false) onBack();
      } catch (saveError) {
        setError(saveError?.message || '保存定时任务失败。');
      }
    });
  };
  return <SettingsEditorPage
    eyebrow="定时任务"
    title={isNew ? '新增任务' : '编辑 ' + task.title}
    description={isNew ? '配置任务内容、调度时间和上下文模式。' : '普通保存保留下一次运行时间；需要重新计算时开启重新计时。'}
    onBack={onBack}
    actions={<><div className="settings-editor-secondary-actions">{isNew ? null : <><button type="button" className="danger" onClick={() => onDelete(task.id)} disabled={saving}>删除任务</button><button type="button" className="secondary" onClick={() => onRun(task.id)} disabled={saving || task.running}>{task.running ? '运行中…' : '立即运行'}</button><button type="button" className="secondary" onClick={() => onViewRuns(task)} disabled={saving}>会话记录</button></>}</div><div className="settings-editor-primary-actions"><button type="button" className="secondary" onClick={onBack} disabled={saving}>取消</button><button type="submit" form="scheduledTaskEditorForm" disabled={saving}>{saving ? '保存中…' : isNew ? '新增任务' : '保存修改'}</button></div></>}
  >
    <form id="scheduledTaskEditorForm" className="settings-editor-form" onSubmit={submit}>
      <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>任务内容</b><p>每次触发时会把任务提示词发送给模型。</p></div></div>
        <label>任务标题<input value={values.title} onChange={event => update('title', event.target.value)} required /></label>
        <label>任务提示词<textarea rows="12" value={values.prompt} onChange={event => update('prompt', event.target.value)} required /></label>
      </section>
      <section className="settings-editor-section">
        <div className="settings-editor-section-head"><div><b>调度计划</b><p>只显示当前调度类型需要填写的字段。</p></div></div>
        <div className="settings-editor-grid">
          <label>调度类型<select value={values.schedule_type} onChange={event => update('schedule_type', event.target.value)}><option value="once">一次性</option><option value="interval">按分钟间隔</option><option value="cron">重复计划</option></select></label>
          <label>上下文模式<select value={values.context_mode} onChange={event => update('context_mode', event.target.value)}><option value="stateless">每次独立执行，最省 token</option><option value="last_result">带上次运行结果</option><option value="session">连续会话，保留完整上下文</option></select></label>
        </div>
        {values.schedule_type === 'once' ? <label>运行时间<input type="datetime-local" value={values.run_at} onChange={event => update('run_at', event.target.value)} required /></label> : null}
        {values.schedule_type === 'interval' ? <label>间隔分钟数<input type="number" min="1" step="1" value={values.interval_minutes} onChange={event => update('interval_minutes', event.target.value)} required /></label> : null}
        {values.schedule_type === 'cron' ? <div className="settings-editor-field"><span>重复计划</span><ScheduleBuilder value={values.cron_schedule} onChange={value => update('cron_schedule', value)} /></div> : null}
        {isNew ? null : <label className="settings-editor-check"><input type="checkbox" checked={!!values.reschedule} onChange={event => update('reschedule', event.target.checked)} /><span><b>保存后重新计时</b><small>开启后按当前时间重新计算下一次运行。</small></span></label>}
      </section>
      {task?.next_run_at ? <div className="settings-editor-status">下次运行：{fmtTime(task.next_run_at)}</div> : null}
      {error ? <div className="backup-health warn" role="alert">{error}</div> : null}
    </form>
  </SettingsEditorPage>;
}
