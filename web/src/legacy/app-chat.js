// ChatDock legacy chat：会话列表、消息渲染、流式输出和 SSE 解析。
async function loadSessions() {
  const list = await api('/api/sessions');
  const q = ((document.getElementById('sessionSearch') || {}).value || '').trim().toLowerCase();
  const filtered = q ? list.filter(s => String(s.title || '').toLowerCase().includes(q)) : list;
  sessions.innerHTML = filtered.map(s => '<div class="session ' + (current===s.id?'active':'') + '" data-action="session-open" data-id="' + dataAttr(s.id) + '">' +
    '<div class="session-title">' + escapeHtml(s.title) + '</div>' +
    '<div class="session-meta">' + s.count + ' 条 · ' + fmtTime(s.updated_at) + '</div>' +
  '</div>').join('');
}

async function newSession() {
  const s = await api('/api/sessions', {method:'POST', body:'{}'});
  current = s.id;
  await loadSessions();
  await renderSession(s);
  closeSidebarOnMobile();
}

async function openSession(id) {
  current = id;
  const s = await api('/api/sessions/' + id);
  await loadSessions();
  await renderSession(s);
  closeSidebarOnMobile();
}

async function renameCurrent() {
  if (!current) return;
  const values = await showFormDialog({
    title: '重命名会话',
    confirmText: '保存标题',
    fields: [{name: 'title', label: '新的会话标题', value: title.textContent || '', required: true}],
  });
  if (!values || !values.title.trim()) return;
  const s = await api('/api/sessions/' + current + '/rename', {method:'POST', body: JSON.stringify({title: values.title.trim()})});
  await renderSession(s);
  await loadSessions();
  showToast('会话标题已保存', 'success');
}

function exportCurrent() {
  if (!current) return;
  window.open(authURL('/api/sessions/' + current + '/export?format=md'), '_blank');
}

async function deleteCurrent() {
  if (!current) return;
  const ok = await showChoice('删除当前会话', '确定删除当前会话？此操作不可恢复。', {confirmText: '删除', danger: true});
  if (!ok) return;
  await api('/api/sessions/' + current, {method:'DELETE'});
  current = null;
  title.textContent = '未选择会话';
  messages.innerHTML = '<div class="empty">创建一个会话，然后开始聊天。</div>';
  await loadSessions();
  showToast('会话已删除', 'success');
}

async function renderSession(s) {
  title.textContent = s.title || '新会话';
  if (!s.messages || s.messages.length === 0) {
    messages.innerHTML = '<div class="empty">这个会话还没有消息。</div>';
    return;
  }
  messages.innerHTML = s.messages.map(renderMessage).join('');
  messages.scrollTop = messages.scrollHeight;
}

function setStreamControls(active) {
  pauseStream.hidden = !active;
  stopStream.hidden = !active;
  pauseStream.disabled = !active;
  stopStream.disabled = !active;
  stopStream.textContent = '中断';
  send.disabled = active;
  continueBtn.disabled = active;
  if (!active) {
    activeAbortController = null;
    activeAssistantEl = null;
    activeReasoningEl = null;
    activeAnswerEl = null;
    streamPaused = false;
    pendingDelta = '';
    pendingReasoning = '';
    activeAnswerBuffer = '';
    activeReasoningBuffer = '';
    pauseStream.textContent = '暂停';
    pauseStream.title = '暂停显示输出';
    pauseStream.setAttribute('aria-label', pauseStream.title);
  }
}

function toggleStreamPause() {
  if (!busy) return;
  streamPaused = !streamPaused;
  pauseStream.textContent = streamPaused ? '继续' : '暂停';
  pauseStream.title = streamPaused ? '继续显示输出' : '暂停显示输出';
  pauseStream.setAttribute('aria-label', pauseStream.title);
  if (!streamPaused) {
    if (pendingReasoning && activeReasoningEl) {
      appendReasoning(pendingReasoning);
      pendingReasoning = '';
    }
    if (pendingDelta && activeAnswerEl) {
      appendAnswer(pendingDelta);
      pendingDelta = '';
    }
    messages.scrollTop = messages.scrollHeight;
  }
}

function stopStreaming() {
  if (!busy || !activeAbortController) return;
  stopStream.disabled = true;
  stopStream.textContent = '中断中';
  activeAbortController.abort();
}

function sendQuickMessage(text) {
  if (busy) return;
  input.value = text;
  sendMsg();
}

function renderMessage(m) {
  const role = m.role || '';
  const content = m.content || '';
  if (role === 'assistant') {
    return '<div class="msg assistant markdown">' + renderMarkdown(content) + '</div>';
  }
  return '<div class="msg ' + escapeHtml(role) + '">' + escapeHtml(content) + '</div>';
}

function appendReasoning(text) {
  if (!text || !activeReasoningEl) return;
  activeReasoningBuffer += text;
  activeReasoningEl.classList.add('show');
  const contentEl = activeReasoningEl.querySelector('.reasoning-content');
  if (contentEl) contentEl.innerHTML = renderMarkdown(activeReasoningBuffer);
}

function appendAnswer(text) {
  if (!text || !activeAnswerEl) return;
  activeAnswerBuffer += text;
  activeAnswerEl.innerHTML = renderMarkdown(activeAnswerBuffer);
}

async function sendMsg() {
  if (busy) return;
  if (!current) await newSession();
  const text = input.value.trim();
  if (!text) return;
  input.value = '';
  busy = true;
  activeAbortController = new AbortController();
  setStreamControls(true);

  const assistantId = 'assistant-' + Date.now();
  const reasoningId = assistantId + '-reasoning';
  const answerId = assistantId + '-answer';
  messages.innerHTML += '<div class="msg user">' + escapeHtml(text) + '</div>' +
    '<div class="msg assistant" id="' + assistantId + '">' +
      '<div class="reasoning" id="' + reasoningId + '"><div class="reasoning-title">思考</div><div class="reasoning-content markdown"></div></div>' +
      '<div class="answer markdown" id="' + answerId + '"></div>' +
    '</div>';
  const assistantEl = document.getElementById(assistantId);
  activeAssistantEl = assistantEl;
  activeReasoningEl = document.getElementById(reasoningId);
  activeAnswerEl = document.getElementById(answerId);
  activeReasoningBuffer = '';
  activeAnswerBuffer = '';
  messages.scrollTop = messages.scrollHeight;

  try {
    const res = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: authHeaders({'Content-Type': 'application/json'}),
      body: JSON.stringify({session_id: current, message: text}),
      signal: activeAbortController.signal
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || res.statusText);
    }

    let finalSession = null;
    await readSSE(res, (event, data) => {
      if (event === 'delta') {
        const reasoning = data.reasoning_content || '';
        const content = data.content || '';
        if (streamPaused) {
          pendingReasoning += reasoning;
          pendingDelta += content;
        } else {
          if (reasoning) appendReasoning(reasoning);
          if (content) appendAnswer(content);
          messages.scrollTop = messages.scrollHeight;
        }
      } else if (event === 'tool_call_start') {
        if (activeAssistantEl) activeAssistantEl.insertAdjacentHTML('beforeend', renderToolEvent('start', data));
      } else if (event === 'tool_call_result') {
        if (activeAssistantEl) activeAssistantEl.insertAdjacentHTML('beforeend', renderToolEvent('result', data));
      } else if (event === 'run_event') {
        if (activeAssistantEl) activeAssistantEl.insertAdjacentHTML('beforeend', renderRunTimelineEvent(data));
      } else if (event === 'run_finish') {
        loadRuns().catch(() => {});
        loadAgentTasks().catch(() => {});
      } else if (event === 'done') {
        finalSession = data.session;
      } else if (event === 'error') {
        throw new Error(data.message || 'stream error');
      }
    });

    if (finalSession) {
      pendingDelta = '';
      pendingReasoning = '';
      await loadSessions();
      await Promise.allSettled([loadRuns(), loadAgentTasks()]);
    }
  } catch (e) {
    if (activeAbortController && activeAbortController.signal.aborted) {
      if (pendingReasoning) {
        appendReasoning(pendingReasoning);
        pendingReasoning = '';
      }
      if (pendingDelta) {
        appendAnswer(pendingDelta);
        pendingDelta = '';
      }
      appendAnswer((activeAnswerBuffer ? '\n\n' : '') + '【已中断】');
      messages.scrollTop = messages.scrollHeight;
      await loadSessions().catch(() => {});
    } else {
      assistantEl.textContent = '错误：' + e.message;
    }
  } finally {
    busy = false;
    setStreamControls(false);
  }
}

async function readSSE(res, onEvent) {
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const {value, done} = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, {stream: true});
    const parts = buffer.split('\n\n');
    buffer = parts.pop() || '';
    for (const part of parts) {
      const event = parseSSE(part);
      if (event) onEvent(event.event, event.data);
    }
  }
  if (buffer.trim()) {
    const event = parseSSE(buffer);
    if (event) onEvent(event.event, event.data);
  }
}

function parseSSE(block) {
  let event = 'message';
  const dataLines = [];
  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim();
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trim());
  }
  if (!dataLines.length) return null;
  return {event, data: JSON.parse(dataLines.join('\n'))};
}

input.addEventListener('keydown', e => {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMsg(); }
});
