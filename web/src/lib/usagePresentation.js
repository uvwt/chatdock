export function currentSessionUsageSummary(messages = []) {
  const summary = {input_tokens: 0, output_tokens: 0, reasoning_tokens: 0, cache_hit_tokens: 0, cache_miss_tokens: 0, total_tokens: 0, reply_count: 0, missing_count: 0, status: '供应商未提供'};
  (Array.isArray(messages) ? messages : []).filter(message => message?.role === 'assistant').forEach(message => {
    const usage = message.usage;
    if (!usage) { summary.missing_count += 1; return; }
    summary.reply_count += 1;
    ['input_tokens', 'output_tokens', 'reasoning_tokens', 'cache_hit_tokens', 'cache_miss_tokens', 'total_tokens'].forEach(key => { summary[key] += Number(usage[key] || 0); });
  });
  if (!summary.reply_count && !summary.missing_count) return null;
  if (summary.reply_count) {
    summary.status = '对话模型已上报用量';
    const totalCache = summary.cache_hit_tokens + summary.cache_miss_tokens;
    if (totalCache) summary.cache_hit_rate = summary.cache_hit_tokens / totalCache;
  }
  return summary;
}
