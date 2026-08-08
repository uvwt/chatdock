const statusOrder = { blocked: 0, active: 1, completed: 2 };
const liveTaskPollIntervalMS = 2000;
const idleTaskPollIntervalMS = 15000;

export function normalizeAgentTask(task = {}) {
  const steps = Array.isArray(task.steps) ? task.steps.map(normalizeAgentTaskStep) : [];
  const completedStepCount = numberOrZero(task.completed_step_count, steps.filter(step => step.status === 'completed').length);
  const stepCount = numberOrZero(task.step_count, steps.length);
  const currentStep = normalizeCurrentStep(task.current_step, steps);
  return {
    ...task,
    id: String(task.id || ''),
    title: String(task.title || task.goal || '未命名任务'),
    goal: String(task.goal || ''),
    status: normalizeAgentTaskStatus(task.status),
    phase: String(task.phase || ''),
    summary: String(task.summary || ''),
    blocker: String(task.blocker || ''),
    completed_step_count: Math.min(completedStepCount, stepCount || completedStepCount),
    step_count: stepCount,
    current_step: currentStep,
    steps,
    updated_at: task.updated_at || '',
  };
}

export function normalizeAgentTaskDetail(payload = {}) {
  return normalizeAgentTask(payload.task || payload);
}

export function sortAgentTasks(tasks = []) {
  return tasks.map(normalizeAgentTask).sort((left, right) => {
    const statusDiff = (statusOrder[left.status] ?? 9) - (statusOrder[right.status] ?? 9);
    if (statusDiff !== 0) return statusDiff;
    return Date.parse(right.updated_at || 0) - Date.parse(left.updated_at || 0);
  });
}

export function agentTaskProgress(task) {
  const normalized = normalizeAgentTask(task);
  const total = normalized.step_count;
  const completed = Math.min(normalized.completed_step_count, total || normalized.completed_step_count);
  return {
    completed,
    total,
    percent: total > 0 ? Math.round((completed / total) * 100) : (normalized.status === 'completed' ? 100 : 0),
    text: total > 0 ? `${completed}/${total}` : (normalized.status === 'completed' ? '已完成' : '进行中'),
  };
}

export function agentTaskStatusMeta(status) {
  switch (normalizeAgentTaskStatus(status)) {
    case 'blocked':
      return { label: '已阻塞', tone: 'blocked' };
    case 'completed':
      return { label: '已完成', tone: 'completed' };
    default:
      return { label: '进行中', tone: 'active' };
  }
}

export function agentTaskStepMeta(status) {
  switch (status) {
    case 'completed':
      return { label: '已完成', tone: 'completed' };
    case 'in_progress':
      return { label: '进行中', tone: 'active' };
    default:
      return { label: '未开始', tone: 'pending' };
  }
}

export function activeAgentTaskCount(tasks = []) {
  return tasks.reduce((count, task) => count + (task.status === 'active' || task.status === 'blocked' ? 1 : 0), 0);
}

export function agentTaskPollInterval(task, busy = false) {
  return busy || task?.status === 'active' || task?.status === 'blocked'
    ? liveTaskPollIntervalMS
    : idleTaskPollIntervalMS;
}

function normalizeAgentTaskStep(step = {}) {
  return {
    ...step,
    id: String(step.id || ''),
    title: String(step.title || step.id || '未命名步骤'),
    status: ['pending', 'in_progress', 'completed'].includes(step.status) ? step.status : 'pending',
  };
}

function normalizeCurrentStep(currentStep, steps) {
  if (currentStep && typeof currentStep === 'object') return normalizeAgentTaskStep(currentStep);
  return steps.find(step => step.status === 'in_progress') || steps.find(step => step.status === 'pending') || null;
}

function normalizeAgentTaskStatus(status) {
  return ['active', 'blocked', 'completed'].includes(status) ? status : 'active';
}

function numberOrZero(value, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback;
}
