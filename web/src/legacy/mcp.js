async function testMCP() {
  try {
    const data = await api('/api/mcp/test');
    if (data.ok) {
      alert('MCP 连接正常：' + data.server + '，工具数 ' + data.tool_count);
    } else {
      alert('MCP 连接失败：' + (data.error || 'unknown error'));
    }
  } catch (e) {
    alert('MCP 测试失败：' + e.message);
  }
}

function renderToolEvent(kind, data) {
  const name = escapeHtml(data.tool || 'tool');
  const suffix = kind === 'start' ? '开始调用' : (data.ok ? '调用完成' : '调用失败');
  return '<div class="tool-event">🔧 ' + suffix + '：' + name + '</div>';
}
