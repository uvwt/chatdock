import { fetchWithAuth } from './http.js';

export function fetchSessions(api) {
  return api('/api/sessions');
}

export function searchSessions(api, query) {
  return api('/api/sessions/search?q=' + encodeURIComponent(query));
}

export function fetchContextPreview(api, id) {
  return api('/api/sessions/' + encodeURIComponent(id) + '/context-preview');
}

export function createSessionRecord(api) {
  return api('/api/sessions', {method:'POST', body:'{}'});
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
