export function streamStatusText(stats = {}, elapsed = 0) {
  const labels = { connecting: '连接模型中', streaming: '流式输出中', paused: '已暂停，后台继续接收', stopping: '正在中断', done: '已完成', error: '输出失败' };
  const parts = [labels[stats.state] || '待命'];
  if (elapsed) parts.push(elapsed + 's');
  if (stats.chars) parts.push(stats.chars + ' 字');
  if (stats.tools) parts.push(stats.tools + ' 个工具');
  if (stats.events) parts.push(stats.events + ' 个事件');
  if (stats.error) parts.push(stats.error);
  return parts.join(' · ');
}

export function attachmentLooksLikeImage(item) {
  return String(item?.mime_type || item?.type || '').toLowerCase().startsWith('image/');
}

export function readableChatError(error, hasImageAttachment = false) {
  const raw = String(error?.message || error || '').trim();
  if (!raw) return '模型调用失败。';
  if (/only support text input|model only support text|不支持.*图片|只支持文本/i.test(raw)) {
    return hasImageAttachment
      ? '当前模型只支持文本输入，不能读取图片。请切换支持图片/视觉的模型，或移除图片附件后再发送。'
      : '当前模型只支持文本输入，不能处理这次请求里的非文本内容。';
  }
  const jsonStart = raw.indexOf('{');
  if (jsonStart >= 0) {
    try {
      const data = JSON.parse(raw.slice(jsonStart));
      const message = data?.error?.message || data?.message || '';
      if (message) return String(message);
    } catch { }
  }
  return raw.replace(/^model api failed:\s*/i, '');
}

export function streamErrorMessage(data = {}) {
  return String(data.message || '模型响应中断。').trim();
}

export function chatErrorDetails(error, message = readableChatError(error)) {
  return {
    message,
    raw: String(error?.raw || error?.message || error || '').trim(),
    code: String(error?.code || '').trim(),
    request_id: String(error?.request_id || '').trim(),
    retryable: Boolean(error?.retryable),
  };
}

export function finalAssistantMessageFromSession(session) {
  const messages = session?.messages || [];
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messages[i]?.role === 'assistant') return messages[i];
  }
  return null;
}

