function fallbackAssistantText(message, streaming) {
  if (streaming) return String(message?.answer || '');
  return String(message?.content || message?.answer || '');
}

function pendingConfirmation(event) {
  return event?.kind === 'confirm' && event.status !== 'resolved';
}

function appendToolBlock(blocks, event) {
  const last = blocks[blocks.length - 1];
  if (last?.kind === 'tools') {
    last.events.push(event);
    return;
  }
  blocks.push({kind: 'tools', events: [event]});
}

function insertBeforeFirstText(blocks, block) {
  const index = blocks.findIndex(item => item.kind === 'text');
  if (index < 0) blocks.push(block);
  else blocks.splice(index, 0, block);
}

// 执行摘要只展示用户能理解的动作名；内部工具标识仍保留在详情原始数据中。
const TOOL_LABELS = {
  agentdock_context: '加载设备上下文',
  browser_act: '操作浏览器',
  browser_session: '启动浏览器',
  browser_snapshot: '截取页面',
  desktop_screenshot: '截取屏幕',
  exec_command: '执行命令',
  file_edit: '修改文件',
  file_publish: '发布文件',
  git_read: '查看 Git',
  git_write: '更新 Git',
  list_dir: '查看目录',
  list_files: '查看文件',
  mcp_manage: '管理 MCP',
  mcp_tool_inspect: '查看工具定义',
  private_note_manage: '管理私密笔记',
  read_file: '读取文件',
  recall_bootstrap: '加载记忆上下文',
  recall_maintain: '维护记忆',
  recall_read: '读取记忆',
  recall_search: '搜索记忆',
  recall_write: '更新记忆',
  search_text: '搜索文本',
  session_act: '控制命令会话',
  session_observe: '查看命令状态',
  skill_package: '管理 Skill',
  task_manage: '管理任务',
  view_image: '查看图片',
  workflow_template_manage: '管理任务模板',
  write_file: '写入文件',
};

function rawToolName(event) {
  const details = event?.details || {};
  const data = details.data || {};
  return String(
    details.arguments?.name ||
    data.arguments?.name ||
    data.result?.tool ||
    details.tool ||
    data.tool ||
    event?.text ||
    event?.meta ||
    ''
  ).trim();
}

function toolNameKey(value) {
  const normalized = String(value || '')
    .replace(/^(正在|已)?调用工具[:：]\s*/, '')
    .replace(/^调用(完成|失败)?[:：]\s*/, '')
    .trim();
  return normalized.split(/__|[./:]/).filter(Boolean).pop()?.toLowerCase() || '';
}

function fallbackToolLabel(key) {
  if (/search|find/.test(key)) return '搜索信息';
  if (/read|fetch|get/.test(key)) return '读取信息';
  if (/list|inspect|status|show/.test(key)) return '查看状态';
  if (/write|edit|update|patch|replace/.test(key)) return '更新内容';
  if (/delete|remove/.test(key)) return '删除内容';
  if (/create|add/.test(key)) return '创建内容';
  if (/exec|run|command/.test(key)) return '执行命令';
  if (/browser|page|screen/.test(key)) return '操作浏览器';
  return '调用工具';
}

export function toolEventDisplayName(event) {
  const raw = rawToolName(event);
  const key = toolNameKey(raw);
  if (!key) return '';
  const readableText = raw
    .replace(/^(正在|已)?调用工具[:：]\s*/, '')
    .replace(/^调用(完成|失败)?[:：]\s*/, '')
    .trim();
  if (/\p{Script=Han}/u.test(readableText)) return readableText;
  return TOOL_LABELS[key] || fallbackToolLabel(key);
}

export function toolEventMetaText(event) {
  const meta = String(event?.meta || '').replace(/^关键词：/, '').trim();
  if (meta.startsWith('{')) {
    try {
      const value = JSON.parse(meta);
      const keys = value && typeof value === 'object' ? Object.keys(value) : [];
      if (keys.length && keys.every(key => ['tool', 'phase', 'status'].includes(key))) return '';
    } catch {
      // 非 JSON 文本继续按普通元信息展示。
    }
  }
  if (meta && toolNameKey(meta) !== toolNameKey(rawToolName(event))) return meta;
  const details = event?.details || {};
  const query = details.arguments?.query || details.data?.arguments?.query || details.data?.result?.query;
  return query ? String(query).trim() : '';
}

export function assistantMessageBlocks(message, { streaming = false, hideThinking = false } = {}) {
  const parts = Array.isArray(message?.parts) ? message.parts : [];
  const blocks = [];
  let hasTextPart = false;
  let hasReasoningPart = false;
  let hasToolPart = false;

  // parts 是流式协议记录的真实时间线；这里只合并相邻工具，不改变思考、工具和正文的先后顺序。
  for (const part of parts) {
    if (part?.kind === 'text' && part.text) {
      hasTextPart = true;
      blocks.push({kind: 'text', text: String(part.text)});
      continue;
    }
    if (part?.kind === 'reasoning' && part.text) {
      hasReasoningPart = true;
      if (!hideThinking) blocks.push({kind: 'reasoning', text: String(part.text)});
      continue;
    }
    if (part?.kind === 'tool' && part.event) {
      hasToolPart = true;
      if (pendingConfirmation(part.event)) blocks.push({kind: 'confirmation', event: part.event});
      else if (toolEventDisplayName(part.event)) appendToolBlock(blocks, part.event);
    }
  }

  const fallbackText = fallbackAssistantText(message, streaming);
  const sourceEvents = Array.isArray(message?.events) ? message.events : [];
  const confirmations = sourceEvents.filter(pendingConfirmation);
  const fallbackEvents = sourceEvents.filter(event => !pendingConfirmation(event) && toolEventDisplayName(event));

  // 历史消息可能只有部分 parts。缺失字段只在对应类型完全不存在时回退，避免重复展示。
  if (!hideThinking && !hasReasoningPart && message?.reasoning) {
    blocks.unshift({kind: 'reasoning', text: String(message.reasoning)});
  }
  if (!hasToolPart && fallbackEvents.length) {
    insertBeforeFirstText(blocks, {kind: 'tools', events: fallbackEvents});
  }
  if (!hasTextPart && fallbackText) {
    blocks.push({kind: 'text', text: fallbackText});
  }
  for (const event of confirmations) {
    if (!blocks.some(block => block.kind === 'confirmation' && block.event === event)) {
      insertBeforeFirstText(blocks, {kind: 'confirmation', event});
    }
  }

  return {
    blocks,
    textParts: blocks.filter(block => block.kind === 'text').map(block => block.text),
  };
}

export function executionBlockSummary(block, { streaming = false } = {}) {
  if (block?.kind === 'reasoning') {
    return {
      label: streaming ? '正在思考' : '思考过程',
      meta: streaming ? '执行中' : '',
      tone: streaming ? 'running' : 'reasoning',
    };
  }

  const events = Array.isArray(block?.events) ? block.events : [];
  const failed = events.filter(event => event?.phase === 'error').length;
  const running = [...events].reverse().find(event => event?.phase === 'running');
  const names = [...new Set(events.map(toolEventDisplayName).filter(Boolean))];

  if (streaming && running) {
    const currentTool = toolEventDisplayName(running);
    return {
      label: currentTool ? `正在${currentTool}` : '正在调用工具',
      meta: events.length > 1 ? `本阶段 ${events.length} 项` : '',
      tone: 'running',
    };
  }

  const label = failed
    ? (events.length === 1 ? `${names[0] || '工具调用'}失败` : `${failed} 项工具失败`)
    : (events.length === 1 ? (names[0] || '工具调用') : `执行 ${events.length} 项工具`);
  const nameSummary = names.slice(0, 2).join('、') + (names.length > 2 ? ' 等' : '');
  return {
    label,
    meta: events.length > 1 ? nameSummary : '',
    tone: failed ? 'error' : 'done',
  };
}
