#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const distAssets = path.join(root, 'web/dist/assets');
const budgets = {
  cssBytes: 340 * 1024,
  jsBytes: 540 * 1024,
};

if (!fs.existsSync(distAssets)) {
  console.error('bundle size: web/dist/assets not found; run npm --prefix web run build first');
  process.exit(1);
}

let cssBytes = 0;
let jsBytes = 0;
for (const file of fs.readdirSync(distAssets)) {
  const size = fs.statSync(path.join(distAssets, file)).size;
  if (file.endsWith('.css')) cssBytes += size;
  if (file.endsWith('.js')) jsBytes += size;
}

const failures = [];
if (cssBytes > budgets.cssBytes) failures.push(`CSS bundle ${cssBytes} exceeds budget ${budgets.cssBytes}`);
if (jsBytes > budgets.jsBytes) failures.push(`JS bundle ${jsBytes} exceeds budget ${budgets.jsBytes}`);
if (failures.length) {
  console.error(failures.map(item => `bundle size: ${item}`).join('\n'));
  process.exit(1);
}
console.log(`bundle size ok: css=${cssBytes}/${budgets.cssBytes}, js=${jsBytes}/${budgets.jsBytes}`);
