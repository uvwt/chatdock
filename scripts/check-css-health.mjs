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

// 悬浮输入框不能通过消息区底部 padding 预留空间，否则 iPhone 安全区会留下白底；
// 但末尾必须有按真实遮挡高度计算的可滚动留白，保证最后正文能完全越过悬浮控件。
const mobileShell = read('web/src/styles/art-direction/05-mobile-shell.css');
const floatingMessagesRule = mobileShell.match(/html:not\(\.chatdock-keyboard-open\) #app\.app:not\(\.chat-empty\) \.messages\s*\{([^}]*)\}/)?.[1] || '';
const floatingMessagesEndRule = mobileShell.match(/html:not\(\.chatdock-keyboard-open\) #app\.app:not\(\.chat-empty\) \.messages::after\s*\{([^}]*)\}/)?.[1] || '';
if (!/padding-bottom:\s*0\s*!important/.test(floatingMessagesRule)
  || !/scroll-padding-bottom:\s*var\(--chatdock-message-bottom-clearance/.test(floatingMessagesRule)
  || !/display:\s*block/.test(floatingMessagesEndRule)
  || !/height:\s*var\(--chatdock-message-bottom-clearance/.test(floatingMessagesEndRule)) {
  failures.push('mobile floating composer must keep the canvas edge-to-edge and add dynamic end clearance for covered content');
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

// 手机端默认隐藏模型入口；聚焦输入区或模型面板打开后才展示，且收起态不能留下可点击的模型按钮。
const composerLayout = read('web/src/styles/composer-layout.css');
const expandedComposerSelector = '.composer:is(.composer-model-picker-open, :focus-within):not(.composer-streaming)';
const collapsedModelLabelRule = composerLayout.match(/#app\.app\s+\.composer:not\(\.composer-streaming\)\s+\.model-picker-label\s*\{([^}]*)\}/)?.[1] || '';
const collapsedModelPickerRule = composerLayout.match(/#app\.app\s+\.composer:not\(\.composer-streaming\)\s+\.model-picker\s*\{([^}]*)\}/)?.[1] || '';
if (!composerLayout.includes(expandedComposerSelector)
  || !/display:\s*none\s*!important/.test(collapsedModelPickerRule)
  || !/display:\s*none\s*!important/.test(collapsedModelLabelRule)) {
  failures.push('composer must hide the model entry until focus, then keep it visible while the picker is open');
}

const desktopComposerStart = composerLayout.indexOf('@media (min-width: 721px) {');
const desktopComposerEnd = composerLayout.indexOf('@media (max-width: 720px) {', desktopComposerStart);
const desktopComposerLayout = desktopComposerStart >= 0 && desktopComposerEnd > desktopComposerStart
  ? composerLayout.slice(desktopComposerStart, desktopComposerEnd)
  : '';
const desktopStableComposerRule = desktopComposerLayout.match(/#app\.app \.composer:not\(\.composer-streaming\),\s*#app\.app \.composer:is\(\.composer-model-picker-open, :focus-within\):not\(\.composer-streaming\)\s*\{([^}]*)\}/)?.[1] || '';
const desktopModelPickerRule = desktopComposerLayout.match(/#app\.app \.composer:not\(\.composer-streaming\) \.model-picker,\s*#app\.app \.composer:is\(\.composer-model-picker-open, :focus-within\):not\(\.composer-streaming\) \.model-picker\s*\{([^}]*)\}/)?.[1] || '';
const desktopModelTriggerRule = desktopComposerLayout.match(/#app\.app \.composer:not\(\.composer-streaming\) \.model-picker-trigger,\s*#app\.app \.composer:is\(\.composer-model-picker-open, :focus-within\):not\(\.composer-streaming\) \.model-picker-trigger\s*\{([^}]*)\}/)?.[1] || '';
const desktopModelLabelRule = desktopComposerLayout.match(/#app\.app \.composer:not\(\.composer-streaming\) \.model-picker-label\s*\{([^}]*)\}/)?.[1] || '';
const desktopModelPopoverRule = desktopComposerLayout.match(/#app\.app \.composer:not\(\.composer-streaming\) \.model-picker-popover\s*\{([^}]*)\}/)?.[1] || '';
const desktopComposerShellRule = desktopComposerLayout.match(/#app\.app:not\(\.chat-empty\) \.composer-shell\s*\{([^}]*)\}/)?.[1] || '';
const desktopMessagesRule = desktopComposerLayout.match(/#app\.app:not\(\.chat-empty\) \.messages\s*\{([^}]*)\}/)?.[1] || '';
const desktopMessagesEndRule = desktopComposerLayout.match(/#app\.app:not\(\.chat-empty\) \.messages::after\s*\{([^}]*)\}/)?.[1] || '';
const emptyConversationLayout = read('web/src/styles/ui-consistency.css');
const emptyMessagesRule = emptyConversationLayout.match(/html:not\(\.chatdock-keyboard-open\) #app\.app\.chat-empty \.messages\s*\{([^}]*)\}/)?.[1] || '';
const emptyComposerShellRule = emptyConversationLayout.match(/html:not\(\.chatdock-keyboard-open\) #app\.app\.chat-empty \.composer-shell\s*\{([^}]*)\}/)?.[1] || '';
if (!/grid-template-areas:\s*"attach input model send"/.test(desktopStableComposerRule)
  || !/height:\s*62px\s*!important/.test(desktopStableComposerRule)
  || !/max-height:\s*62px\s*!important/.test(desktopStableComposerRule)
  || !/border-radius:\s*30px\s*!important/.test(desktopStableComposerRule)
  || !/display:\s*flex\s*!important/.test(desktopModelPickerRule)
  || !/grid-area:\s*model\s*!important/.test(desktopModelPickerRule)
  || !/justify-self:\s*end/.test(desktopModelPickerRule)
  || !/justify-content:\s*flex-end\s*!important/.test(desktopModelTriggerRule)
  || !/display:\s*inline-flex\s*!important/.test(desktopModelLabelRule)
  || !/right:\s*0/.test(desktopModelPopoverRule)
  || !/left:\s*auto/.test(desktopModelPopoverRule)
  || !/position:\s*absolute\s*!important/.test(desktopComposerShellRule)
  || !/background:\s*transparent\s*!important/.test(desktopComposerShellRule)
  || !/pointer-events:\s*none\s*!important/.test(desktopComposerShellRule)
  || !/padding-bottom:\s*0\s*!important/.test(desktopMessagesRule)
  || !/scroll-padding-bottom:\s*168px/.test(desktopMessagesRule)
  || !/display:\s*block/.test(desktopMessagesEndRule)
  || !/height:\s*168px/.test(desktopMessagesEndRule)
  || !/transform:\s*translateY\(-96px\)/.test(emptyMessagesRule)
  || !/position:\s*relative/.test(emptyComposerShellRule)
  || !/grid-row:\s*2/.test(emptyComposerShellRule)
  || !/align-self:\s*center/.test(emptyComposerShellRule)
  || !/transform:\s*none/.test(emptyComposerShellRule)) {
  failures.push('desktop composer must stay single-line with an always-visible model and preserve floating or centered placement');
}

const composerGlassRule = composerLayout.match(/#app\.app\s+\.composer\s*\{([^}]*)\}/)?.[1] || '';
const standardComposerTextareaRule = composerLayout.match(/#app\.app\s+\.composer:not\(\.composer-streaming\)\s+textarea\s*\{([^}]*)\}/)?.[1] || '';
const streamingComposerTextareaRule = composerLayout.match(/#app\.app\s+\.composer\.composer-streaming\s+textarea\s*\{([^}]*)\}/)?.[1] || '';
const composerTextareaRules = [standardComposerTextareaRule, streamingComposerTextareaRule];
if (!/-webkit-backdrop-filter:\s*blur\(/.test(composerGlassRule)
  || !/backdrop-filter:\s*blur\(/.test(composerGlassRule)
  || !/background:\s*var\(--composer-glass-bg\)\s*!important/.test(composerGlassRule)
  || composerTextareaRules.some(rule => !/background:\s*transparent\s*!important/.test(rule)
    || !/border:\s*0\s*!important/.test(rule)
    || !/box-shadow:\s*none\s*!important/.test(rule))) {
  failures.push('normal and streaming composers must share one frosted glass shell with transparent textareas');
}

const composerActionRule = composerLayout.match(/#app\.app\s+\.composer:not\(\.composer-streaming\)\s+:is\(\.attach-control,\s*\.model-picker-trigger,\s*#send\)\s*\{([^}]*)\}/)?.[1] || '';
if (!/border:\s*0\s*!important/.test(composerActionRule)
  || !/background:\s*transparent\s*!important/.test(composerActionRule)
  || !/box-shadow:\s*none\s*!important/.test(composerActionRule)) {
  failures.push('composer inner actions must remain frameless so the outer composer is the only visible container');
}

const chatCanvasStyles = read('web/src/styles/art-direction/02-canvas.css');
const messageCanvasRule = chatCanvasStyles.match(/#app\.app \.messages\s*\{([^}]*)\}/)?.[1] || '';
if (!/scroll-behavior:\s*auto/.test(messageCanvasRule)
  || /scroll-behavior:\s*smooth/.test(messageCanvasRule)) {
  failures.push('message history must jump to its initial position without a page-length smooth scroll');
}

const chatErrorStyles = read('web/src/styles/chat/03-chat.css');
const chatErrorCardRule = chatErrorStyles.match(/\.chat-error-card\s*\{([^}]*)\}/)?.[1] || '';
const chatErrorMessageRule = chatErrorStyles.match(/\.chat-error-message\s*\{([^}]*)\}/)?.[1] || '';
const chatErrorMetaRule = chatErrorStyles.match(/\.chat-error-meta\s*\{([^}]*)\}/)?.[1] || '';
const chatErrorMetaTextRule = chatErrorStyles.match(/\.chat-error-meta > small,\s*\.chat-error-details > small\s*\{([^}]*)\}/)?.[1] || '';
if (!/width:\s*min\(720px,\s*100%\)/.test(chatErrorCardRule)
  || !/color:\s*var\(--gpt-text\)/.test(chatErrorCardRule)
  || !/grid-template-columns:\s*18px\s+minmax\(0,\s*1fr\)/.test(chatErrorCardRule)
  || !/color:\s*var\(--gpt-text\)/.test(chatErrorMessageRule)
  || !/line-height:\s*1\.5/.test(chatErrorMessageRule)
  || !/display:\s*flex/.test(chatErrorMetaRule)
  || !/flex-wrap:\s*wrap/.test(chatErrorMetaRule)
  || !/color:\s*var\(--gpt-muted\)/.test(chatErrorMetaTextRule)) {
  failures.push('chat error notice must stay compact and use theme-aware readable text colors');
}

const visualCoherence = read('web/src/styles/visual-coherence.css');
const sidebarTitleRule = visualCoherence.match(/#app\.app \.sidebar-section-title,\s*#app\.app \.sidebar-section-head > button,\s*#app\.app \.sidebar-tree-node > summary span,\s*#app\.app \.sidebar-tree-session,\s*#app\.app #sessions \.session-title\s*\{([^}]*)\}/)?.[1] || '';
const sidebarEmphasisRule = visualCoherence.match(/#app\.app \.sidebar-section-title-emphasis,\s*#app\.app \.sidebar-section-head > \.sidebar-section-title-emphasis\s*\{([^}]*)\}/)?.[1] || '';
if (!/color:\s*var\(--gpt-text\)\s*!important/.test(sidebarTitleRule)
  || !/font-size:\s*var\(--ui-font-body\)\s*!important/.test(sidebarTitleRule)
  || !/font-weight:\s*400\s*!important/.test(sidebarTitleRule)
  || !/line-height:\s*1\.35\s*!important/.test(sidebarTitleRule)
  || !/letter-spacing:\s*0\s*!important/.test(sidebarTitleRule)
  || !/text-transform:\s*none\s*!important/.test(sidebarTitleRule)) {
  failures.push('sidebar project, task, and conversation titles must share the regular typography rule');
}
if (!/font-weight:\s*600\s*!important/.test(sidebarEmphasisRule)) {
  failures.push('pinned, projects, and all conversations section titles must use the emphasized weight');
}

const transparentTopbarRule = visualCoherence.match(/#app\.app\s+\.topbar\s*\{([^}]*)\}/)?.[1] || '';
if (!/border:\s*0/.test(transparentTopbarRule)
  || !/background:\s*transparent/.test(transparentTopbarRule)
  || !/box-shadow:\s*none/.test(transparentTopbarRule)
  || !/-webkit-backdrop-filter:\s*none/.test(transparentTopbarRule)
  || !/backdrop-filter:\s*none/.test(transparentTopbarRule)) {
  failures.push('desktop topbar must remain borderless and fully transparent');
}

const wideCanvasTopbarStart = visualCoherence.indexOf('@container chat-canvas (min-width: 1340px) {');
const wideCanvasTopbarEnd = visualCoherence.indexOf('/* Conversation and composer */', wideCanvasTopbarStart);
const wideCanvasTopbar = wideCanvasTopbarStart >= 0 && wideCanvasTopbarEnd > wideCanvasTopbarStart
  ? visualCoherence.slice(wideCanvasTopbarStart, wideCanvasTopbarEnd)
  : '';
if (!/position:\s*absolute/.test(wideCanvasTopbar)
  || !/pointer-events:\s*none/.test(wideCanvasTopbar)
  || !/padding-top:\s*16px/.test(wideCanvasTopbar)
  || !/display:\s*none/.test(wideCanvasTopbar)) {
  failures.push('wide desktop topbar must use the side gutters without reserving a full content row');
}

// 悬浮控件不能被主画布的通用层级规则改回相对定位，否则会重新占据主轴高度。
const shellLayout = read('web/src/styles/art-direction/01-shell.css');
const floatingSafeSelector = '#app.app main > :not(.topbar):not(.jump-latest):not(.current-session-task):not(.composer-shell)';
const mainContentLayerRule = shellLayout.match(/#app\.app\s+main\s*>\s*:not\(\.topbar\):not\(\.jump-latest\):not\(\.current-session-task\):not\(\.composer-shell\)\s*\{([^}]*)\}/)?.[1] || '';
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
