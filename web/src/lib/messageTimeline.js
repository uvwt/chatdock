const MESSAGE_TIME_GAP_MS = 30 * 60 * 1000;

function parseMessageTime(value) {
  if (!value) return null;
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function sameLocalDay(left, right) {
  return left.getFullYear() === right.getFullYear()
    && left.getMonth() === right.getMonth()
    && left.getDate() === right.getDate();
}

export function shouldShowMessageTimeDivider(previousMessage, message, gapMS = MESSAGE_TIME_GAP_MS) {
  const current = parseMessageTime(message?.created_at);
  if (!current) return false;

  const previous = parseMessageTime(previousMessage?.created_at);
  // 首条消息提供整段会话的起始时间；后续消息再按时间间隔决定是否重复显示。
  if (!previous) return true;
  if (current < previous) return false;

  // 跨日期时即使间隔不足半小时也显示，避免午夜附近的消息失去日期上下文。
  return !sameLocalDay(previous, current) || current.getTime() - previous.getTime() >= gapMS;
}

export function formatMessageTimeDivider(value, nowValue = Date.now()) {
  const date = parseMessageTime(value);
  const now = parseMessageTime(nowValue);
  if (!date || !now) return '';

  const time = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
  if (sameLocalDay(date, now)) return time;

  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
  if (sameLocalDay(date, yesterday)) return `昨天 ${time}`;

  const monthDay = `${date.getMonth() + 1}月${date.getDate()}日 ${time}`;
  return date.getFullYear() === now.getFullYear() ? monthDay : `${date.getFullYear()}年${monthDay}`;
}
