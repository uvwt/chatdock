async function testMCP() {
  try {
    const data = await api('/api/mcp/test');
    if (data.ok) {
      showToast('MCP 连接正常：' + data.server + '，工具数 ' + data.tool_count, 'success');
    } else {
      showToast('MCP 连接失败：' + (data.error || 'unknown error'), 'error');
    }
  } catch (e) {
    showToast('MCP 测试失败：' + e.message, 'error');
  }
}

function renderToolEvent(kind, data) {
  const name = escapeHtml(data.tool || 'tool');
  const suffix = kind === 'start' ? '开始调用' : (data.ok ? '调用完成' : '调用失败');
  return '<div class="tool-event">🔧 ' + suffix + '：' + name + '</div>';
}

function renderRunTimelineEvent(data) {
  const label = escapeHtml(data.summary || data.tool || 'MCP 工具事件');
  const status = escapeHtml(runStatusLabel(data.status || ''));
  const meta = [data.server, data.action, fmtDuration(data.duration_ms)].filter(Boolean).join(' · ');
  return '<div class="tool-event run-event-inline">🧭 ' + label + '<div class="tool-event-meta">' + status + (meta ? ' · ' + escapeHtml(meta) : '') + '</div></div>';
}
