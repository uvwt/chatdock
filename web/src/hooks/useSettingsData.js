import { useCallback, useState } from 'react';
import { fetchConfig, fetchDataStatus, fetchMCPConfig, fetchMCPStatus, fetchModelProviders, fetchScheduledTasks, fetchSetupStatus, fetchSystemStatus, fetchWorkspaces } from '../lib/settingsApi.js';

const defaultConfig = {
  base_url: '',
  api_key: '',
  model: '',
  models: [],
  system_prompt: '',
  context_mode: 'auto',
  max_context_messages: 12,
  temperature: 0.7,
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
  const [workspaceSummaries, setWorkspaceSummaries] = useState([]);
  const [scheduledTasks, setScheduledTasks] = useState([]);
  const [dataStatus, setDataStatus] = useState(null);
  const [systemStatus, setSystemStatus] = useState(null);
  const [mcpStatus, setMcpStatus] = useState([]);
  const [workspacePromptPreview, setWorkspacePromptPreview] = useState('');
  const [mcpConfig, setMcpConfig] = useState('');
  const [config, setConfig] = useState(defaultConfig);

  const loadWorkspaceSummaries = useCallback(async () => {
    const data = await fetchWorkspaces(api);
    const workspaceRows = data.workspaces || [];
    setWorkspaces(workspaceRows);
    // 侧栏只需要轻量工作空间摘要；这里统一由 Workspace API 派生，避免再出现 prompts 双轨概念。
    setWorkspaceSummaries(workspaceRows.map(ws => ({
      name: ws.name || ws.id,
      active: !!ws.active,
      created_at: ws.created_at,
      updated_at: ws.updated_at,
      count: ws.session_count || 0,
    })));
    return data;
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
    workspaceSummaries,
    setWorkspaceSummaries,
    scheduledTasks,
    setScheduledTasks,
    dataStatus,
    setDataStatus,
    systemStatus,
    setSystemStatus,
    mcpStatus,
    setMcpStatus,
    workspacePromptPreview,
    setWorkspacePromptPreview,
    mcpConfig,
    setMcpConfig,
    config,
    setConfig,
    loadWorkspaceSummaries,
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
