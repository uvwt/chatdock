#!/usr/bin/env node
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const files = [
  'web/src/pageHtml.js',
  ...fs.readdirSync(path.join(root, 'web/src/legacy'))
    .filter(name => name.endsWith('.js'))
    .map(name => `web/src/legacy/${name}`),
];

const source = files.map(file => fs.readFileSync(path.join(root, file), 'utf8')).join('\n');
const literalActions = new Set();
for (const match of source.matchAll(/data-action="([^"]+)"/g)) {
  const value = match[1];
  if (!value.includes("'") && !value.includes('+') && !value.includes('dataAttr(')) literalActions.add(value);
}

const clickActions = new Set();
const handlerSource = fs.readFileSync(path.join(root, 'web/src/legacy/app-ui.js'), 'utf8');
for (const match of handlerSource.matchAll(/'([^']+)':/g)) clickActions.add(match[1]);

const changeActions = new Set(['prompt-select', 'skill-toggle', 'task-toggle']);
const inputActions = new Set(['session-search', 'skill-search', 'task-search']);
const submitActions = new Set(['login-submit']);
const handled = new Set([...clickActions, ...changeActions, ...inputActions, ...submitActions]);
const missing = [...literalActions].filter(action => !handled.has(action)).sort();

if (missing.length) {
  console.error('Unhandled data-action values:');
  for (const action of missing) console.error(`- ${action}`);
  process.exit(1);
}
console.log(`data-action ok: ${literalActions.size} literal actions covered`);
