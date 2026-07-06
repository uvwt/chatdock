import { useCallback, useState } from 'react';
import { fetchConfig, fetchDataStatus, fetchMCPConfig, fetchMCPStatus, fetchModelProviders, fetchPrompts, fetchScheduledTasks, fetchSetupStatus, fetchSystemStatus, fetchWorkspaces } from '../lib/settingsApi.js';

const defaultConfig = {
  base_url: '',
  api_key: '',
  model: '',
  models: [],
  system_prompt: '',
  context_mode: 'auto',
  max_context_messages: 12,
  temperature: 0.7,
  enable_thinking: false,
  hide_thinking: false,
  has_api_key: false,
  embedding_base_url: '',
  embedding_api_key: '',
  embedding_model: 'BAAI/bge-m3',
  has_embedding_api_key: false,
};

function configFromServer(c = {}) {
  return {
    provider_id: c.provider_id || '',
    base_url: c.base_url || '',
    api_key: '',
    model: c.model || '',
    models: Array.isArray(c.models) ? c.models : [],
    system_prompt: c.system_prompt || '',
    context_mode: c.context_mode || 'auto',
    max_context_messages: c.max_context_messages || 12,
    temperature: c.temperature ?? 0.7,
    enable_thinking: !!c.enable_thinking,
    hide_thinking: !!c.hide_thinking,
    has_api_key: !!c.has_api_key,
    embedding_base_url: c.embedding_base_url || '',
    embedding_api_key: '',
    embedding_model: c.embedding_model || 'BAAI/bge-m3',
    has_embedding_api_key: !!c.has_embedding_api_key,
  };
}

export function useSettingsData(api) {
  const [setupStatus, setSetupStatus] = useState(null);
  const [workspaces, setWorkspaces] = useState([]);
  const [providers, setProviders] = useState([]);
  const [prompts, setPrompts] = useState([]);
  const [scheduledTasks, setScheduledTasks] = useState([]);
  const [dataStatus, setDataStatus] = useState(null);
  const [systemStatus, setSystemStatus] = useState(null);
  const [mcpStatus, setMcpStatus] = useState([]);
  const [promptPreview, setPromptPreview] = useState('');
  const [mcpConfig, setMcpConfig] = useState('');
  const [config, setConfig] = useState(defaultConfig);

  const loadPrompts = useCallback(async () => {
    const data = await fetchPrompts(api);
    setPrompts(data.prompts || []);
  }, [api]);

  const loadConfig = useCallback(async () => {
    const c = await fetchConfig(api);
    setConfig(configFromServer(c));
  }, [api]);

  const loadMCPConfig = useCallback(async () => {
    const c = await fetchMCPConfig(api);
    setMcpConfig(c.content || '{\n  "servers": {}\n}\n');
  }, [api]);

  const loadSetupStatus = useCallback(async () => {
    const data = await fetchSetupStatus(api);
    setSetupStatus(data);
  }, [api]);

  const loadWorkspaces = useCallback(async () => {
    const data = await fetchWorkspaces(api);
    setWorkspaces(data.workspaces || []);
  }, [api]);

  const loadModelProviders = useCallback(async () => {
    const data = await fetchModelProviders(api);
    setProviders(data.providers || []);
  }, [api]);

  const loadScheduledTasks = useCallback(async () => {
    const data = await fetchScheduledTasks(api);
    setScheduledTasks(data.tasks || []);
  }, [api]);

  const loadDataStatus = useCallback(async () => {
    const data = await fetchDataStatus(api);
    setDataStatus(data);
  }, [api]);

  const loadSystemStatus = useCallback(async () => {
    const data = await fetchSystemStatus(api);
    setSystemStatus(data);
  }, [api]);

  const loadMCPStatus = useCallback(async () => {
    const data = await fetchMCPStatus(api);
    setMcpStatus(data.servers || []);
  }, [api]);

  return {
    setupStatus,
    setSetupStatus,
    workspaces,
    setWorkspaces,
    providers,
    setProviders,
    prompts,
    setPrompts,
    scheduledTasks,
    setScheduledTasks,
    dataStatus,
    setDataStatus,
    systemStatus,
    setSystemStatus,
    mcpStatus,
    setMcpStatus,
    promptPreview,
    setPromptPreview,
    mcpConfig,
    setMcpConfig,
    config,
    setConfig,
    loadPrompts,
    loadConfig,
    loadMCPConfig,
    loadSetupStatus,
    loadWorkspaces,
    loadModelProviders,
    loadScheduledTasks,
    loadDataStatus,
    loadSystemStatus,
    loadMCPStatus,
  };
}
