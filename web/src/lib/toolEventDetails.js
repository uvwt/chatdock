import { fmtDuration } from './appUtils.js';

export function hasDisplayValue(value) {
  if (value == null || value === '') return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value).length > 0;
  return true;
}

function arrayValue(value) {
  return Array.isArray(value) ? value : [];
}

export function actualToolCall(details = {}, data = {}) {
  const proxyTool = details.tool || data.tool || '';
  const args = details.arguments ?? data.arguments ?? {};
  const result = details.result ?? data.result;
  if (proxyTool === 'chatdock_tool_execute') {
    const resultObject = result && typeof result === 'object' && !Array.isArray(result) ? result : {};
    const parseError = args?._parse_error || data?._parse_error || '';
    if (parseError) {
      return { proxyTool, actualTool: '工具参数解析失败', actualArguments: args, actualResult: resultObject.result ?? result, parseError, mode: 'execute_parse_error' };
    }
    return { proxyTool, actualTool: args.name || resultObject.tool || '', actualArguments: args.arguments ?? {}, actualResult: resultObject.result ?? result, mode: 'execute' };
  }
  if (proxyTool === 'chatdock_tools_describe') {
    const names = arrayValue(args.names || result?.tools?.map?.(item => item?.name)).filter(Boolean);
    return { proxyTool, actualTool: names.length === 1 ? names[0] : (names.length ? names.length + ' 个工具说明' : ''), actualArguments: { names }, actualResult: result, names, mode: 'describe' };
  }
  if (proxyTool === 'chatdock_tools_search') {
    const candidates = arrayValue(result?.tools).map(item => item?.name || item?.full_name || item).filter(Boolean);
    const count = Number(data.count ?? result?.count ?? candidates.length) || candidates.length;
    return { proxyTool, actualTool: count ? count + ' 个候选工具' : '工具搜索', actualArguments: args, actualResult: result, candidates, candidateCount: count, query: args.query || result?.query || data.query || '', mode: 'search' };
  }
  return { proxyTool: '', actualTool: proxyTool, actualArguments: args, actualResult: result, mode: 'direct' };
}

export function buildToolEventDetail(event) {
  const details = event?.details || {};
  const data = details.data && typeof details.data === 'object' ? details.data : {};
  const eventName = details.event || data.event || 'tool_event';
  const tool = details.tool || data.tool || '';
  const actual = actualToolCall(details, data);
  const ok = typeof details.ok === 'boolean' ? details.ok : (typeof data.ok === 'boolean' ? data.ok : null);
  const failed = ok === false || /error|failed|cancelled/i.test(eventName) || details.error || data.error;
  const ready = /ready|resolved|finish|done/i.test(eventName) || ok === true;
  const status = failed ? '失败' : (ready ? '完成' : '事件');
  const duration = details.duration_ms || data.duration_ms;
  const heading = actual.mode === 'execute_parse_error'
    ? '工具参数 JSON 解析失败'
    : (actual.mode === 'search'
      ? (actual.candidateCount ? '找到 ' + actual.candidateCount + ' 个候选工具' : '工具搜索')
      : (actual.mode === 'describe'
        ? (actual.names?.length ? '查看 ' + actual.names.length + ' 个工具说明' : '工具说明')
        : (actual.actualTool || event?.text || tool || '工具事件')));
  const subheading = [actual.query ? '关键词：' + actual.query : '', actual.proxyTool && actual.proxyTool !== actual.actualTool ? '代理：' + actual.proxyTool : ''].filter(Boolean).join(' · ');
  const metrics = [
    actual.mode === 'search' ? { label: '候选', value: actual.candidateCount || actual.candidates?.length || '' } : null,
    { label: '工具总数', value: data.tool_count ?? details.tool_count },
    { label: '内置工具', value: data.builtin_tool_count ?? details.builtin_tool_count },
    { label: '耗时', value: duration ? fmtDuration(duration) : '' },
  ].filter(item => item && hasDisplayValue(item.value));
  const rows = [
    { label: '事件类型', value: eventName },
    { label: '状态', value: status },
    actual.proxyTool ? { label: '代理工具', value: actual.proxyTool } : null,
    actual.query ? { label: '搜索关键词', value: actual.query } : null,
    actual.parseError ? { label: '解析错误', value: actual.parseError } : null,
    { label: '服务', value: data.server || details.server },
    { label: '动作', value: data.action || details.action },
  ].filter(item => item && hasDisplayValue(item.value));
  const sections = [];
  if (actual.mode === 'search' && actual.candidates?.length) sections.push({ title: '候选工具', value: actual.candidates, display: 'tools', emptyText: '没有候选工具' });
  if (hasDisplayValue(details.error || data.error)) sections.push({ title: '错误', value: details.error || data.error, tone: 'danger' });
  const hasArgumentsSection = hasDisplayValue(actual.actualArguments);
  const hasResultSection = hasDisplayValue(actual.actualResult);
  if (hasArgumentsSection) sections.push({ title: actual.mode === 'execute_parse_error' ? '模型原始参数' : (actual.mode === 'execute' ? '参数' : '请求参数'), value: actual.actualArguments, emptyText: '无参数' });
  if (hasResultSection) sections.push({ title: actual.mode === 'execute' ? '响应' : '完整响应', value: actual.actualResult, emptyText: '无响应', collapsed: hasArgumentsSection });
  if (hasDisplayValue(data) && !sections.length) sections.push({ title: '事件数据', value: data });
  sections.push({ title: '原始事件', value: details, collapsed: true, muted: true });
  return {
    event: eventName,
    heading,
    subheading,
    status,
    statusTone: failed ? 'danger' : (ready ? 'success' : ''),
    primary: actual.actualTool ? { label: actual.mode === 'execute_parse_error' ? '失败原因' : (actual.mode === 'execute' ? '实际调用' : (actual.mode === 'search' ? '搜索结果' : '说明对象')), name: actual.actualTool, hint: subheading || (actual.proxyTool ? '通过 ' + actual.proxyTool : '') } : null,
    metrics,
    rows,
    sections,
  };
}
