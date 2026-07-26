import React, { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { compactModelName, providerLabel } from '../lib/modelProviderForm.js';
import { ChevronDown, Sparkles } from './icons.js';

export function ComposerModelPicker({ busy, providers, selectedProvider, selectedModel, showSelection, open, setOpen, selectModel, openSettings }) {
  const pickerRef = useRef(null);
  const popoverRef = useRef(null);
  const [mobileSheet, setMobileSheet] = useState(() => typeof window !== 'undefined' && window.matchMedia('(max-width: 720px)').matches);

  useEffect(() => {
    const media = window.matchMedia('(max-width: 720px)');
    const updateLayout = () => setMobileSheet(media.matches);
    updateLayout();
    media.addEventListener('change', updateLayout);
    return () => media.removeEventListener('change', updateLayout);
  }, []);

  useEffect(() => {
    if (!open) return undefined;

    // 模型选择器不是阻塞式弹窗：点击组件外部或按 Esc 即可收起，内部操作保持正常。
    const closeOnOutsidePointer = event => {
      if (!pickerRef.current?.contains(event.target) && !popoverRef.current?.contains(event.target)) setOpen(false);
    };
    const closeOnEscape = event => {
      if (event.key === 'Escape') setOpen(false);
    };

    document.addEventListener('pointerdown', closeOnOutsidePointer);
    window.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('pointerdown', closeOnOutsidePointer);
      window.removeEventListener('keydown', closeOnEscape);
    };
  }, [open, setOpen]);

  const popover = <div className="model-picker-popover" ref={popoverRef} role="dialog" aria-label="选择供应商模型">
    <div className="model-picker-head"><b>选择供应商模型</b></div>
    <div className="model-provider-list">
      {providers.length ? providers.map(provider => <div className="model-provider-item" key={provider.choice_id}>
        <div className="model-provider-title"><b>{providerLabel(provider)}</b><small>{provider.base_url || '-'}</small></div>
        <div className="model-chip-list">
          {provider.models.length ? provider.models.map(name => <button type="button" key={provider.choice_id + name} className={'model-chip ' + (selectedProvider?.choice_id === provider.choice_id && selectedModel === name ? 'active' : '')} onClick={() => selectModel(provider, name)}>{name}</button>) : <button type="button" className="model-chip" onClick={() => openSettings('model')}>添加模型</button>}
        </div>
      </div>) : <div className="empty compact">还没有可用模型，请先到配置中心手动添加。</div>}
    </div>
  </div>;

  return <div className="model-picker" ref={pickerRef}>
    <button
      type="button"
      className="secondary model-picker-trigger"
      disabled={busy || !providers.length}
      onClick={() => setOpen(value => !value)}
      title={showSelection ? `切换模型：${providerLabel(selectedProvider)} · ${selectedModel}` : '选择模型'}
      aria-label={showSelection ? `切换模型：${selectedModel}` : '选择模型'}
      aria-haspopup="dialog"
      aria-expanded={open}
    >
      <Sparkles className="model-picker-icon" size={17} aria-hidden="true" />
      {showSelection ? <span className="model-picker-label"><b>{compactModelName(selectedModel)}</b><ChevronDown size={14} aria-hidden="true" /></span> : null}
    </button>
    {open ? (mobileSheet ? createPortal(popover, document.body) : popover) : null}
  </div>;
}
