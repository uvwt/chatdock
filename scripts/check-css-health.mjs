#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const stylesDir = path.join(root, 'web/src/styles');
const failures = [];
const budgets = {
  exactDuplicateBlocks: 32,
  selectorsRepeatedAtLeast4: 76,
  maxSelectorLayers: 24,
  maxCssFileLines: 650,
};

function read(file) { return fs.readFileSync(path.join(root, file), 'utf8'); }
function normalize(text) { return text.trim().replace(/\s+/g, ' '); }
function listCSS(dir) {
  const out = [];
  for (const item of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, item.name);
    if (item.isDirectory()) out.push(...listCSS(full));
    else if (item.isFile() && item.name.endsWith('.css')) out.push(full);
  }
  return out;
}

function scopedKey(file, offset) {
  const before = file.text.slice(0, offset);
  const contexts = [];
  for (const match of before.matchAll(/@(?:media|supports|container)[^{]+\{/g)) {
    const start = match.index;
    const rest = before.slice(start);
    const opens = (rest.match(/\{/g) || []).length;
    const closes = (rest.match(/\}/g) || []).length;
    if (opens > closes) contexts.push(normalize(match[0].slice(0, -1)));
  }
  if (!contexts.length) return 'base';
  return contexts.slice(-2).join(' > ');
}

for (const file of listCSS(stylesDir)) {
  const rel = path.relative(root, file);
  const lineCount = fs.readFileSync(file, 'utf8').split(/\n/).length;
  if (lineCount > budgets.maxCssFileLines) failures.push(`CSS file ${rel} has ${lineCount} lines, exceeds budget ${budgets.maxCssFileLines}`);
}

const settingsEntry = read('web/src/styles/settings.css');
for (const legacyName of ['final-layout', 'visual-polish', 'override']) {
  if (settingsEntry.includes(legacyName)) failures.push(`settings.css must use semantic module names, not ${legacyName}`);
}

const exactBlocks = new Map();
const selectorFiles = new Map();
for (const filename of listCSS(stylesDir)) {
  const rel = path.relative(root, filename);
  const file = { text: fs.readFileSync(filename, 'utf8') };
  for (const match of file.text.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selector = normalize(match[1]);
    const body = normalize(match[2]);
    if (!selector || !body || selector.startsWith('@')) continue;
    const targetsSessionList = selector.split(',').some(item => item.trim().endsWith('#sessions'));
    if (targetsSessionList && /\boverflow(?:-y)?\s*:\s*visible\b/.test(body)) {
      failures.push(`session list must remain scrollable; ${rel} sets visible overflow on ${selector}`);
    }
    const scope = scopedKey(file, match.index);
    const blockKey = scope + '\n' + selector + '\n' + body;
    const selectorKey = scope + '\n' + selector;
    exactBlocks.set(blockKey, [...(exactBlocks.get(blockKey) || []), rel]);
    selectorFiles.set(selectorKey, [...(selectorFiles.get(selectorKey) || []), rel]);
  }
}

const duplicateBlockCount = [...exactBlocks.values()].filter(items => items.length > 1).length;
const repeatedSelectorCount = [...selectorFiles.values()].filter(items => items.length >= 4).length;
if (duplicateBlockCount > budgets.exactDuplicateBlocks) failures.push(`CSS exact duplicate block count ${duplicateBlockCount} exceeds budget ${budgets.exactDuplicateBlocks}`);
if (repeatedSelectorCount > budgets.selectorsRepeatedAtLeast4) failures.push(`CSS selectors repeated >=4 count ${repeatedSelectorCount} exceeds budget ${budgets.selectorsRepeatedAtLeast4}`);

for (const [selector, files] of selectorFiles) {
  if (selector.includes('.settings-page') && files.length > budgets.maxSelectorLayers) {
    failures.push(`settings selector layer count ${files.length} exceeds budget ${budgets.maxSelectorLayers}: ${selector}`);
  }
}

if (failures.length) {
  console.error(failures.map(item => `css health: ${item}`).join('\n'));
  process.exit(1);
}
console.log(`css health ok: exact_duplicate_blocks=${duplicateBlockCount}/${budgets.exactDuplicateBlocks}, repeated_selectors>=4=${repeatedSelectorCount}/${budgets.selectorsRepeatedAtLeast4}, max_selector_layers<=${budgets.maxSelectorLayers}, max_css_file_lines<=${budgets.maxCssFileLines}`);
