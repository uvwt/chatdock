#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const stylesDir = path.join(root, 'web/src/styles');
const failures = [];
const budgets = {
  exactDuplicateBlocks: 4,
  selectorsRepeatedAtLeast4: 48,
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
  for (const match of before.matchAll(/@(?:media|supports|container|(?:-webkit-)?keyframes)[^{]+\{/g)) {
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

// 移动端聊天页会锁住 body；配置页必须解除锁并使用浏览器原生文档滚动。
// 禁止配置页再次变成固定高度的嵌套滚动容器，iOS Safari 对这类结构容易吞掉触摸滚动。
const settingsLayout = read('web/src/styles/settings/15-layout-system.css');
const settingsPageShellRule = settingsLayout.match(/\.settings-page\s+\.settings\s*\{([^}]*)\}/)?.[1] || '';
if (!/position:\s*static\s*!important/.test(settingsPageShellRule)) {
  failures.push('settings page shell must reset the legacy fixed drawer positioning');
}
if (!/top:\s*auto\s*!important/.test(settingsPageShellRule)
  || !/right:\s*auto\s*!important/.test(settingsPageShellRule)
  || !/bottom:\s*auto\s*!important/.test(settingsPageShellRule)
  || !/left:\s*auto\s*!important/.test(settingsPageShellRule)) {
  failures.push('settings page shell must reset all legacy drawer edges');
}

const mobileSettingsLayout = read('web/src/styles/settings/16-mobile-layout.css');
const mobileDocumentScrollRule = mobileSettingsLayout.match(/html\.settings-page-visible,[^{]*\{([^}]*)\}/)?.[1] || '';
if (!/height:\s*auto\s*!important/.test(mobileDocumentScrollRule)
  || !/overflow-y:\s*auto\s*!important/.test(mobileDocumentScrollRule)) {
  failures.push('mobile settings mode must restore native document scrolling');
}

const mobileSettingsPageRule = mobileSettingsLayout.match(/\.settings-page\s*\{([^}]*)\}/)?.[1] || '';
if (!/height:\s*auto\s*!important/.test(mobileSettingsPageRule)
  || !/overflow:\s*visible\s*!important/.test(mobileSettingsPageRule)) {
  failures.push('mobile settings page must expand naturally inside the document');
}
if (/overflow-y:\s*auto/.test(mobileSettingsPageRule)
  || /height:\s*var\(--chatdock-viewport-height/.test(mobileSettingsPageRule)) {
  failures.push('mobile settings page must not create a nested viewport scroller');
}

const mobileSettingsContentRule = mobileSettingsLayout.match(/\.settings-page\s+\.settings-content\s*\{([^}]*)\}/)?.[1] || '';
if (!/display:\s*block\s*!important/.test(mobileSettingsContentRule)
  || !/height:\s*auto\s*!important/.test(mobileSettingsContentRule)
  || !/overflow:\s*visible\s*!important/.test(mobileSettingsContentRule)) {
  failures.push('mobile settings content must participate in the native document flow');
}

const mobileActiveModuleRule = mobileSettingsLayout.match(/\.settings-page\s+\.module-view\.active\s*\{([^}]*)\}/)?.[1] || '';
if (!/height:\s*auto\s*!important/.test(mobileActiveModuleRule)
  || !/overflow:\s*visible\s*!important/.test(mobileActiveModuleRule)) {
  failures.push('mobile active settings module must not clip long content');
}

// 会话列表只允许纵向滚动。连续 URL、Token 或 API Key 必须在消息自身换行，
// 不能把整张消息画布撑成横向滚动容器，否则 iOS 会出现整页偏移和左侧裁切。
const conversationCanvas = read('web/src/styles/art-direction/02-canvas.css');
const messagesRule = conversationCanvas.match(/#app\.app\s+\.messages\s*\{([^}]*)\}/)?.[1] || '';
if (!/min-width:\s*0/.test(messagesRule)
  || !/overflow-x:\s*hidden/.test(messagesRule)
  || !/overflow-y:\s*auto/.test(messagesRule)) {
  failures.push('conversation canvas must remain a vertical-only scroller');
}

// 悬浮输入框不能通过消息区底部 padding 预留空间，否则 iPhone 安全区会留下白底。
const mobileShell = read('web/src/styles/art-direction/05-mobile-shell.css');
const floatingMessagesRule = mobileShell.match(/html:not\(\.chatdock-keyboard-open\) #app\.app:not\(\.chat-empty\) \.messages\s*\{([^}]*)\}/)?.[1] || '';
if (!/padding-bottom:\s*0\s*!important/.test(floatingMessagesRule)
  || !/scroll-padding-bottom:\s*calc\(132px\s*\+\s*env\(safe-area-inset-bottom/.test(floatingMessagesRule)) {
  failures.push('mobile floating composer must keep visual content edge-to-edge and reserve only scroll positioning space');
}

const finalMobileMessageRule = mobileShell.match(/html:not\(\.chatdock-keyboard-open\) #app\.app:not\(\.chat-empty\) \.messages > \.msg:last-child\s*\{([^}]*)\}/)?.[1] || '';
if (!/margin-bottom:\s*0\s*!important/.test(finalMobileMessageRule)) {
  failures.push('mobile final message must not leave a blank margin below the floating composer');
}

const conversationContent = read('web/src/styles/art-direction/03-conversation.css');
const userMessageRule = conversationContent.match(/#app\.app\s+\.msg\.user,[^{]*\{([^}]*)\}/)?.[1] || '';
if (!/min-width:\s*0/.test(userMessageRule)
  || !/overflow-wrap:\s*anywhere/.test(userMessageRule)) {
  failures.push('user and system messages must wrap unbroken long content inside the viewport');
}

// 有有效模型时工具栏必须稳定展开，不能用 focus 触发布局变化，否则按下模型按钮会丢失 click。
const composerLayout = read('web/src/styles/composer-layout.css');
const selectedComposerSelector = '.composer.composer-model-selected:not(.composer-streaming)';
const collapsedModelLabelRule = composerLayout.match(/#app\.app\s+\.composer:not\(\.composer-streaming\)\s+\.model-picker-label\s*\{([^}]*)\}/)?.[1] || '';
if (!composerLayout.includes(selectedComposerSelector)
  || composerLayout.includes('.composer-model-picker-open')
  || composerLayout.includes(':focus-within')
  || !/display:\s*none\s*!important/.test(collapsedModelLabelRule)) {
  failures.push('composer model toolbar must remain stable and must not shift on focus or picker open');
}

// 悬浮控件不能被主画布的通用层级规则改回相对定位，否则会重新占据主轴高度。
const shellLayout = read('web/src/styles/art-direction/01-shell.css');
const floatingSafeSelector = '#app.app main > :not(.jump-latest):not(.current-session-task):not(.composer-shell)';
const mainContentLayerRule = shellLayout.match(/#app\.app\s+main\s*>\s*:not\(\.jump-latest\):not\(\.current-session-task\):not\(\.composer-shell\)\s*\{([^}]*)\}/)?.[1] || '';
if (!shellLayout.includes(floatingSafeSelector)
  || !/position:\s*relative/.test(mainContentLayerRule)
  || /#app\.app\s+main\s*>\s*:not\(\.jump-latest\)\s*\{/.test(shellLayout)
  || /#app\.app\s+main\s*>\s*\*/.test(shellLayout)) {
  failures.push('main content layering must exclude jump, task, and composer floating controls');
}
const jumpLatestRule = read('web/src/styles/chat/02-chat.css').match(/\.jump-latest\s*\{([^}]*)\}/)?.[1] || '';
const currentTaskRule = read('web/src/styles/current-task.css').match(/^\.current-session-task\s*\{([^}]*)\}/m)?.[1] || '';
if (!/position:\s*absolute/.test(jumpLatestRule)
  || !/position:\s*absolute/.test(currentTaskRule)) {
  failures.push('jump-to-latest and current-session-task must remain absolutely positioned');
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
