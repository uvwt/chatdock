import React from 'react';

const frequencyOptions = [
  {value: 'daily', label: '每天'},
  {value: 'weekdays', label: '工作日'},
  {value: 'weekly', label: '每周'},
  {value: 'monthly', label: '每月'},
];

const weekdayOptions = [
  {value: '1', label: '星期一'},
  {value: '2', label: '星期二'},
  {value: '3', label: '星期三'},
  {value: '4', label: '星期四'},
  {value: '5', label: '星期五'},
  {value: '6', label: '星期六'},
  {value: '0', label: '星期日'},
];

function normalizedSchedule(value) {
  const schedule = value && typeof value === 'object' ? value : {};
  const times = Array.isArray(schedule.times) ? schedule.times.filter(Boolean) : [];
  return {
    frequency: schedule.frequency || 'daily',
    weekday: String(schedule.weekday || '1'),
    month_day: String(schedule.month_day || '1'),
    times: times.length ? times : ['09:00'],
    timezone: String(schedule.timezone || 'Asia/Shanghai'),
    original_expressions: Array.isArray(schedule.original_expressions) ? schedule.original_expressions : [],
  };
}

export function ScheduleBuilder({value, onChange}) {
  const schedule = normalizedSchedule(value);
  const update = patch => onChange({...schedule, ...patch});
  const updateTime = (index, time) => update({times: schedule.times.map((item, i) => i === index ? time : item)});
  const removeTime = index => update({times: schedule.times.filter((_, i) => i !== index)});
  const addTime = () => update({times: [...schedule.times, '09:00']});
  const changeFrequency = frequency => update({frequency, times: schedule.frequency === 'custom' ? ['09:00'] : schedule.times});

  return <div className="schedule-builder">
    <div className="schedule-builder-grid">
      <label><span>执行周期</span><select value={schedule.frequency} onChange={e => changeFrequency(e.target.value)}>
        {schedule.frequency === 'custom' ? <option value="custom">当前自定义计划</option> : null}
        {frequencyOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
      </select></label>
      {schedule.frequency === 'weekly' ? <label><span>星期</span><select value={schedule.weekday} onChange={e => update({weekday: e.target.value})}>
        {weekdayOptions.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
      </select></label> : null}
      {schedule.frequency === 'monthly' ? <label><span>日期</span><input type="number" min="1" max="31" value={schedule.month_day} required onChange={e => update({month_day: e.target.value})} /></label> : null}
    </div>

    {schedule.frequency === 'custom' ? <div className="schedule-builder-notice">该任务使用了工具创建的高级计划。保持当前选项即可原样保存，选择常用周期后可重新设置。</div> : <>
      <div className="schedule-builder-times-head"><span>执行时间</span><button type="button" className="secondary small" onClick={addTime}>+ 添加时间</button></div>
      <div className="schedule-builder-times">{schedule.times.map((time, index) => <div className="schedule-builder-time" key={index}>
        <input type="time" value={time} required onChange={e => updateTime(index, e.target.value)} />
        <button type="button" className="secondary small" disabled={schedule.times.length <= 1} onClick={() => removeTime(index)}>删除</button>
      </div>)}</div>
    </>}

    <div className="schedule-builder-timezone">按 {schedule.timezone} 时区运行</div>
  </div>;
}
