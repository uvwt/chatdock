export function visibleSessionRows({ sessionSearch = '', sessionSearchResults = [], sessions = [] }) {
  return String(sessionSearch || '').trim() ? sessionSearchResults : sessions;
}
