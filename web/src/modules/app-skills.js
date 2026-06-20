// ChatDock module skills：技能列表、编辑、开关和删除。
async function loadSkills() {
  try {
    const data = await api('/api/skills');
    skillItems = data.skills || [];
    renderSkills();
  } catch (e) {
    skillItems = [];
    renderPanelError(skills, e, 'skills-load');
  }
}

function renderSkills() {
  const q = ((document.getElementById('skillSearch') || {}).value || '').trim().toLowerCase();
  const list = q ? skillItems.filter(s => [s.name, s.description, s.content].some(v => String(v || '').toLowerCase().includes(q))) : skillItems;
  if (!list.length) {
    skills.innerHTML = '<div class="hint">暂无技能。技能会作为当前提示词空间的补充系统指令注入模型请求。</div>';
    return;
  }
  skills.innerHTML = list.map(s => '<div class="skill-card">' +
    '<div class="skill-head">' +
      '<div><div class="skill-name">' + escapeHtml(s.name || '未命名技能') + '</div>' +
      '<div class="skill-desc">' + escapeHtml(s.description || '无描述') + '</div></div>' +
      '<div class="skill-actions">' +
        '<button class="secondary small" data-action="skill-edit" data-id="' + dataAttr(s.id) + '">编辑</button>' +
        '<button class="danger small" data-action="skill-delete" data-id="' + dataAttr(s.id) + '">删除</button>' +
      '</div>' +
    '</div>' +
    '<label class="skill-toggle"><input type="checkbox" data-action="skill-toggle" data-id="' + dataAttr(s.id) + '" ' + (s.enabled ? 'checked' : '') + ' /> 启用</label>' +
  '</div>').join('');
}

function findSkill(id) {
  return skillItems.find(s => s.id === id) || null;
}

async function editSkill(id) {
  if (busy) return;
  const existing = id ? findSkill(id) : null;
  const values = await showFormDialog({
    title: existing ? '编辑技能' : '新增技能',
    confirmText: existing ? '保存技能' : '新增技能',
    fields: [
      {name: 'name', label: '技能名称', value: existing ? existing.name : '', required: true},
      {name: 'description', label: '技能描述（可选）', type: 'textarea', rows: 2, value: existing ? (existing.description || '') : ''},
      {name: 'content', label: '技能内容', type: 'textarea', rows: 8, value: existing ? (existing.content || '') : '', required: true},
    ],
  });
  if (!values) return;
  if (!values.name.trim() || !values.content.trim()) {
    showToast('技能名称和内容不能为空', 'error');
    return;
  }
  const payload = {name: values.name.trim(), description: values.description || '', content: values.content.trim(), enabled: existing ? !!existing.enabled : true};
  const data = await api(existing ? '/api/skills/' + encodeURIComponent(existing.id) : '/api/skills', {method: existing ? 'PUT' : 'POST', body: JSON.stringify(payload)});
  skillItems = data.skills || [];
  renderSkills();
  showToast(existing ? '技能已保存' : '技能已新增', 'success');
}

async function toggleSkill(id, enabled) {
  const existing = findSkill(id);
  if (!existing) return;
  const data = await api('/api/skills/' + encodeURIComponent(id), {method:'PUT', body: JSON.stringify({
    name: existing.name,
    description: existing.description || '',
    content: existing.content || '',
    enabled: !!enabled,
  })});
  skillItems = data.skills || [];
  renderSkills();
}

async function deleteSkill(id) {
  const existing = findSkill(id);
  if (!existing) return;
  const ok = await showChoice('删除技能', '确定删除技能「' + existing.name + '」？此操作不可恢复。', {confirmText: '删除', danger: true});
  if (!ok) return;
  const data = await api('/api/skills/' + encodeURIComponent(id), {method:'DELETE'});
  skillItems = data.skills || [];
  renderSkills();
  showToast('技能已删除', 'success');
}
