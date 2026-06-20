#!/usr/bin/env node
// React 版前端不再依赖 data-action 事件委托。
// 保留这个脚本作为兼容入口，避免旧文档或人工命令直接调用时失败。
console.log('data-action check skipped: React frontend uses component event handlers');
