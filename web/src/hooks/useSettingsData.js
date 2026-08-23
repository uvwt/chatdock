import { useCallback, useMemo, useState } from 'react';
import { fetchConfig, fetchDataStatus, fetchMCPConfig, fetchMCPStatus, fetchModelProviders, fetchProjects, fetchScheduledTasks, fetchSetupStatus, fetchSystemStatus, normalizeProjectListResponse } from '../lib/settingsApi.js';
import { globalConfigDraftChanged, mcpConfigDraftChanged } from '../lib/settingsDraft.js';

const defaultConfig = {
  base_url: '',
  api_key: '',
  model: '',
  models: [],
  fallback_provider_id: '',
  fallback_model: '',
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
    fallback_provider_id: c.fallback_provider_id || '',
    fallback_model: c.fallback_model || '',
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
  const [projects, setProjects] = useState([]);
  const [projectsLoaded, setProjectsLoaded] = useState(false);
  const [projectSessionCounts, setProjectSessionCounts] = useState({all: 0, plain: 0, byProject: {}});
  const [providers, setProviders] = useState([]);
  const [scheduledTasks, setScheduledTasks] = useState([]);
  // 侧栏“定时任务”分段需要区分未加载与真的没有任务，否则数据到达时整段会把下方内容推走。
  const [scheduledTasksLoaded, setScheduledTasksLoaded] = useState(false);
  const [dataStatus, setDataStatus] = useState(null);
  const [systemStatus, setSystemStatus] = useState(null);
  const [mcpStatus, setMcpStatus] = useState([]);
  const [projectPromptPreview, setProjectPromptPreview] = useState('');
  const [mcpConfig, setMcpConfig] = useState('');
  const [savedMCPConfig, setSavedMCPConfig] = useState(null);
  const [builtinTools, setBuiltinTools] = useState([]);
  const [config, setConfig] = useState(defaultConfig);
  const [configLoaded, setConfigLoaded] = useState(false);
  const [savedConfig, setSavedConfig] = useState(null);

  const configDirty = useMemo(() => globalConfigDraftChanged(config, savedConfig), [config, savedConfig]);
  const mcpConfigDirty = useMemo(() => mcpConfigDraftChanged(mcpConfig, savedMCPConfig), [mcpConfig, savedMCPConfig]);

  const loadConfig = useCallback(async () => {
    const c = await fetchConfig(api);
    const next = configFromServer(c);
    setConfig(next);
    setSavedConfig(next);
    setConfigLoaded(true);
  }, [api]);

  const loadMCPConfig = useCallback(async () => {
    const c = await fetchMCPConfig(api);
    const content = c.content || '{\n  "builtin_tools": {\n    "tool_exposure": "direct"\n  },\n  "servers": {}\n}\n';
    setMcpConfig(content);
    setSavedMCPConfig(content);
    setBuiltinTools(c.builtin_tools || []);
  }, [api]);

  const loadSetupStatus = useCallback(async () => {
    const data = await fetchSetupStatus(api);
    setSetupStatus(data);
  }, [api]);

  const loadProjects = useCallback(async () => {
    try {
      const data = await fetchProjects(api);
      const next = normalizeProjectListResponse(data);
      setProjects(next.projects);
      setProjectSessionCounts(next.sessionCounts);
      return data;
    } finally {
      // 失败也要退出加载态，否则侧栏项目分段会永久停在骨架上。
      setProjectsLoaded(true);
    }
  }, [api]);

  const loadModelProviders = useCallback(async () => {
    const data = await fetchModelProviders(api);
    setProviders(data.providers || []);
  }, [api]);

  const loadScheduledTasks = useCallback(async () => {
    try {
      const data = await fetchScheduledTasks(api);
      setScheduledTasks(data.tasks || []);
    } finally {
      // 失败也要退出加载态，否则侧栏定时任务分段会永久停在骨架上。
      setScheduledTasksLoaded(true);
    }
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

  const discardSettingsDrafts = useCallback(() => {
    if (savedConfig) setConfig(savedConfig);
    if (savedMCPConfig != null) setMcpConfig(savedMCPConfig);
  }, [savedConfig, savedMCPConfig]);

  return {
    setupStatus,
    setSetupStatus,
    projects,
    projectsLoaded,
    projectSessionCounts,
    providers,
    setProviders,
    scheduledTasks,
    scheduledTasksLoaded,
    setScheduledTasks,
    dataStatus,
    setDataStatus,
    systemStatus,
    setSystemStatus,
    mcpStatus,
    setMcpStatus,
    projectPromptPreview,
    setProjectPromptPreview,
    mcpConfig,
    setMcpConfig,
    builtinTools,
    config,
    configLoaded,
    setConfig,
    configDirty,
    mcpConfigDirty,
    discardSettingsDrafts,
    loadConfig,
    loadMCPConfig,
    loadSetupStatus,
    loadProjects,
    loadModelProviders,
    loadScheduledTasks,
    loadDataStatus,
    loadSystemStatus,
    loadMCPStatus,
  };
}
