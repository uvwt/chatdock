#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const distAssets = path.join(root, 'web/dist/assets');
const budgets = {
  cssBytes: 340 * 1024,
  cssChunkBytes: 210 * 1024,
  jsBytes: 540 * 1024,
  jsChunkBytes: 500 * 1024,
};

if (!fs.existsSync(distAssets)) {
  console.error('bundle size: web/dist/assets not found; run npm --prefix web run build first');
  process.exit(1);
}

let cssBytes = 0;
let cssChunkCount = 0;
let jsBytes = 0;
let jsChunkCount = 0;
let largestCSSChunk = {file: '', size: 0};
let largestJSChunk = {file: '', size: 0};
for (const file of fs.readdirSync(distAssets)) {
  const size = fs.statSync(path.join(distAssets, file)).size;
  if (file.endsWith('.css')) {
    cssBytes += size;
    cssChunkCount++;
    if (size > largestCSSChunk.size) largestCSSChunk = {file, size};
    continue;
  }
  if (!file.endsWith('.js')) continue;

  jsBytes += size;
  jsChunkCount++;
  if (size > largestJSChunk.size) largestJSChunk = {file, size};
}

const failures = [];
if (cssBytes > budgets.cssBytes) failures.push(`CSS bundle ${cssBytes} exceeds budget ${budgets.cssBytes}`);
if (largestCSSChunk.size > budgets.cssChunkBytes) {
  failures.push(`CSS chunk ${largestCSSChunk.file} is ${largestCSSChunk.size}, exceeds budget ${budgets.cssChunkBytes}`);
}
if (jsBytes > budgets.jsBytes) failures.push(`JS bundle ${jsBytes} exceeds budget ${budgets.jsBytes}`);
if (largestJSChunk.size > budgets.jsChunkBytes) {
  failures.push(`JS chunk ${largestJSChunk.file} is ${largestJSChunk.size}, exceeds budget ${budgets.jsChunkBytes}`);
}
if (failures.length) {
  console.error(failures.map(item => `bundle size: ${item}`).join('\n'));
  process.exit(1);
}
console.log(`bundle size ok: css=${cssBytes}/${budgets.cssBytes}, css_chunks=${cssChunkCount}, largest_css=${largestCSSChunk.file}:${largestCSSChunk.size}/${budgets.cssChunkBytes}, js=${jsBytes}/${budgets.jsBytes}, js_chunks=${jsChunkCount}, largest_js=${largestJSChunk.file}:${largestJSChunk.size}/${budgets.jsChunkBytes}`);
