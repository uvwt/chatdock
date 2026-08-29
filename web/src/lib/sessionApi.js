import { fetchWithAuth } from './http.js';

function appendProjectFilter(params, projectFilter = 'all') {
  if (projectFilter === 'plain') params.set('project_id', '');
  else if (projectFilter && projectFilter !== 'all') params.set('project_id', projectFilter);
}

function appendPinnedFilter(params, pinned) {
  if (pinned === true) params.set('pinned', '1');
  else if (pinned === false) params.set('pinned', '0');
}

export function fetchSessions(api, { cursor = '', limit = 30, projectFilter = 'all', pinned } = {}) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (cursor) params.set('cursor', cursor);
  appendProjectFilter(params, projectFilter);
  appendPinnedFilter(params, pinned);
  return api('/api/sessions?' + params.toString());
}

export function fetchPinned(api) {
  return api('/api/pinned');
}

export function searchSessions(api, query, { cursor = '', limit = 30, projectFilter = 'all' } = {}) {
  const params = new URLSearchParams({ q: query, limit: String(limit) });
  if (cursor) params.set('cursor', cursor);
  appendProjectFilter(params, projectFilter);
  return api('/api/sessions/search?' + params.toString());
}

export function fetchProviderSystemPrompt(api, id) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/provider-system-prompt');
}

export function createSessionRecord(api, { projectID = '' } = {}) {
  const body = projectID ? JSON.stringify({project_id: projectID}) : '{}';
  return api('/api/sessions', {method:'POST', body});
}

export function fetchSession(api, id) {
  return api('/api/sessions/' + encodeURIComponent(id));
}

export function fetchSessionToolEvent(api, id, ref = {}) {
  if (ref.event_id) {
    return api('/api/sessions/' + encodeURIComponent(id) + '/tool-events/' + encodeURIComponent(ref.event_id));
  }
  const params = new URLSearchParams();
  if (ref.message_index != null) params.set('message_index', String(ref.message_index));
  if (ref.event_index != null) params.set('event_index', String(ref.event_index));
  if (ref.part_index != null && ref.event_index == null) params.set('part_index', String(ref.part_index));
  return api('/api/sessions/' + encodeURIComponent(id) + '/tool-event?' + params.toString());
}


export function callMCPAppTool(api, sessionID, {sourceTool, name, arguments: args}) {
  return api('/api/mcp/apps/call', {method: 'POST', body: JSON.stringify({session_id: sessionID, source_tool: sourceTool, name, arguments: args || {}})});
}

export async function resolveSessionToolEvent(api, currentSessionID, event, cache = new Map()) {
  if (!event?.details?.lazy) return event;
  const ref = event.details;
  const sessionID = ref.session_id || currentSessionID;
  const eventID = ref.event_id || event.id || '';
  const hasEventIndex = ref.event_index !== undefined && ref.event_index !== null;
  const hasPartIndex = ref.part_index !== undefined && ref.part_index !== null;
  if (!sessionID || (!eventID && (ref.message_index === undefined || (!hasEventIndex && !hasPartIndex)))) return event;
  const cacheKey = eventID ? [sessionID, eventID].join(':') : [sessionID, ref.message_index, hasEventIndex ? ref.event_index : '', hasPartIndex ? ref.part_index : ''].join(':');
  if (!cache.has(cacheKey)) {
    const data = await fetchSessionToolEvent(api, sessionID, ref);
    cache.set(cacheKey, data.event || event);
  }
  return cache.get(cacheKey) || event;
}

export function renameSession(api, id, title) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/rename', {method:'POST', body: JSON.stringify({title})});
}

export function pinSession(api, id, pinned) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/pin', {method:'POST', body: JSON.stringify({pinned})});
}

export function updateSessionModel(api, id, { providerID, model }) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/model', {method:'POST', body: JSON.stringify({provider_id: providerID || '', model: model || ''})});
}

export function cloneSession(api, id) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/clone', {method:'POST', body:'{}'});
}

export function branchSession(api, id, messageIndex) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/branch', {method:'POST', body: JSON.stringify({message_index: messageIndex})});
}

export function editSessionMessage(api, id, {messageIndex, messageID, content}) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/messages', {method:'POST', body: JSON.stringify({message_index: messageIndex, message_id: messageID || '', content})});
}

export function deleteSession(api, id) {
  return api('/api/sessions/' + encodeURIComponent(id), {method:'DELETE'});
}

export function fetchSessionMarkdown(id, authHeaders) {
  return fetchWithAuth('/api/sessions/' + encodeURIComponent(id) + '/export?format=md', {authHeaders});
}
