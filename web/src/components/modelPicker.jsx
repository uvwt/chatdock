import React from 'react';
import { compactModelName, providerLabel } from '../lib/modelProviderForm.js';

export function ComposerModelPicker({ busy, providers = [], selectedProvider, selectedModel, open, setOpen, selectModel, openSettings }) {
  return <div className="model-picker">
    <button type="button" className="secondary model-picker-trigger" disabled={busy || !providers.length} onClick={() => setOpen(value => !value)} title="选择供应商 / 模型"><span>{providerLabel(selectedProvider)}</span><b>{compactModelName(selectedModel) || '未选择模型'}</b></button>
    {open ? <div className="model-picker-popover">
      <div className="model-picker-head"><b>选择供应商模型</b><button type="button" className="secondary small" onClick={() => setOpen(false)}>关闭</button></div>
      <div className="model-provider-list">
        {providers.length ? providers.map(provider => <div className="model-provider-item" key={provider.choice_id}>
          <div className="model-provider-title"><b>{providerLabel(provider)}</b><small>{provider.base_url || '-'}</small></div>
          <div className="model-chip-list">
            {provider.models.length ? provider.models.map(name => <button type="button" key={provider.choice_id + name} className={'model-chip ' + (selectedProvider?.choice_id === provider.choice_id && selectedModel === name ? 'active' : '')} onClick={() => selectModel(provider, name)}>{name}</button>) : <button type="button" className="model-chip" onClick={() => openSettings('model')}>添加模型</button>}
          </div>
        </div>) : <div className="empty compact">还没有可用模型，请先到配置中心手动添加。</div>}
      </div>
    </div> : null}
  </div>;
}
