import { readSSE } from './sse.js';

const reconnectDelays = [0, 250, 500, 1000, 2000, 4000];

export function fetchChatJobs(api, sessionID) {
  return api('/api/chat/jobs?session_id=' + encodeURIComponent(sessionID));
}

export function cancelChatJob(api, jobID) {
  return api('/api/chat/jobs/' + encodeURIComponent(jobID) + '/cancel', {method:'POST'});
}

export function guideChatJob(api, jobID, message) {
  return api('/api/chat/jobs/' + encodeURIComponent(jobID) + '/guide', {method:'POST', body: JSON.stringify({message})});
}

export function resolveMCPConfirmation(api, id, approve) {
  return api('/api/mcp/confirmations/' + encodeURIComponent(id) + '/resolve', {method:'POST', body: JSON.stringify({approve: !!approve})});
}

export async function streamChatJobEvents({jobID, authHeaders, signal, onEvent}) {
  const initialResponse = await fetchJobEventResponse(jobID, 0, authHeaders, signal);
  await consumeJobEventStream({initialResponse, jobID, authHeaders, signal, onEvent});
}

export async function streamChat({authHeaders, signal, sessionID, message, attachmentIDs, providerID, model, regenerate = false, onEvent}) {
  const initialResponse = await fetch('/api/chat/stream', {
    method:'POST',
    headers: authHeaders({'Content-Type':'application/json'}),
    body: JSON.stringify({session_id: sessionID, message, attachment_ids: attachmentIDs, provider_id: providerID || '', model: model || '', regenerate: !!regenerate}),
    signal,
  });
  if (!initialResponse.ok) {
    const data = await initialResponse.json().catch(() => ({}));
    throw requestError(initialResponse, data);
  }
  await consumeJobEventStream({initialResponse, jobID: '', authHeaders, signal, onEvent});
}

async function consumeJobEventStream({initialResponse, jobID, authHeaders, signal, onEvent}) {
  let response = initialResponse;
  let activeJobID = jobID;
  let lastEventID = 0;
  let reconnectIndex = 0;
  let lastError = null;

  const dispatch = (event, data, eventID) => {
    if (event === 'job_started') activeJobID = data.id || activeJobID;
    if (eventID > 0) lastEventID = Math.max(lastEventID, eventID);
    onEvent(event, data);
  };

  while (true) {
    if (!response) {
      if (!activeJobID || reconnectIndex >= reconnectDelays.length) throw lastError || new Error('流式连接已中断');
      await waitForReconnect(reconnectDelays[reconnectIndex], signal);
      reconnectIndex += 1;
      try {
        response = await fetchJobEventResponse(activeJobID, lastEventID, authHeaders, signal);
      } catch (error) {
        if (signal?.aborted) throw error;
        lastError = error;
        continue;
      }
    }

    try {
      await readSSE(response, dispatch);
      return;
    } catch (error) {
      if (signal?.aborted) throw error;
      lastError = error;
      response = null;
    }
  }
}

async function fetchJobEventResponse(jobID, after, authHeaders, signal) {
  const url = '/api/chat/jobs/' + encodeURIComponent(jobID) + '/events?after=' + encodeURIComponent(String(after || 0));
  const response = await fetch(url, {headers: authHeaders(), signal});
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw requestError(response, data);
  }
  return response;
}

function waitForReconnect(delay, signal) {
  if (signal?.aborted) return Promise.reject(signal.reason || new DOMException('Aborted', 'AbortError'));
  if (delay <= 0) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const timer = globalThis.setTimeout(done, delay);
    signal?.addEventListener('abort', aborted, {once: true});

    function done() {
      signal?.removeEventListener('abort', aborted);
      resolve();
    }
    function aborted() {
      globalThis.clearTimeout(timer);
      reject(signal.reason || new DOMException('Aborted', 'AbortError'));
    }
  });
}

function requestError(res, data = {}) {
  const requestID = res.headers.get('X-Request-ID') || data.request_id || '';
  const message = data.error || data.message || res.statusText || '请求失败';
  const err = new Error(message);
  err.request_id = requestID;
  err.status = res.status;
  return err;
}
