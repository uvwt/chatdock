#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const failures = [];
const read = file => fs.readFileSync(path.join(root, file), 'utf8');

const appCss = read('web/src/app.css').trim().split(/\n/).filter(Boolean);
const nonImportLines = appCss.filter(line => !line.startsWith('@import'));
if (nonImportLines.length) failures.push('web/src/app.css must stay import-only; move real rules into web/src/styles/*.css');

const settingsCss = read('web/src/styles/settings.css').trim().split(/\n/).filter(Boolean);
const settingsCssNonImportLines = settingsCss.filter(line => !line.startsWith('@import'));
if (settingsCssNonImportLines.length) failures.push('web/src/styles/settings.css must stay import-only; move real rules into web/src/styles/settings/*.css');

const appSource = read('web/src/App.jsx');
const settingFetchNames = ['fetchConfig', 'fetchDataStatus', 'fetchMCPConfig', 'fetchMCPStatus', 'fetchModelProviders', 'fetchScheduledTasks', 'fetchSetupStatus', 'fetchSystemStatus', 'fetchProjects'];
for (const name of settingFetchNames) {
  if (appSource.includes(name)) failures.push(`settings fetch helper should stay in useSettingsData.js, not App.jsx: ${name}`);
}
if (!read('web/src/hooks/useSettingsData.js').includes('export function useSettingsData')) failures.push('settings data loaders belong in web/src/hooks/useSettingsData.js');

if (!read('web/src/hooks/useAttachments.js').includes('export function useAttachments')) failures.push('attachment state and file upload logic belong in web/src/hooks/useAttachments.js');
if (appSource.includes('uploadFileRequest')) failures.push('file upload request handling should stay in useAttachments.js, not App.jsx');

const settings = read('web/src/components/settings.jsx');
for (const stale of ['function ReplyModule', 'function EmbeddingModule']) {
  if (settings.includes(stale)) failures.push(`stale unused component remains: ${stale}`);
}

const tokenParser = settings.match(/JSON\.parse\(atob\(parts\[(\d+)\]/);
if (!tokenParser || tokenParser[1] !== '1') failures.push('JWT expiry parser must decode payload parts[1], not header parts[0]');

const app = read('web/src/App.jsx');
if (app.includes('function toolEventText') || app.includes('function mergeToolResultEvent')) failures.push('tool event protocol helpers belong in web/src/lib/toolEvents.js, not App.jsx');

const forbiddenAppHelpers = ['ComposerModelPicker', 'readableChatError', 'buildToolEventDetail', 'streamStatusText'];
for (const name of forbiddenAppHelpers) {
  if (app.includes('function ' + name)) failures.push(`helper should stay outside App.jsx: ${name}`);
}
if (!read('web/src/components/modelPicker.jsx').includes('export function ComposerModelPicker')) failures.push('model picker component missing');
if (!read('web/src/lib/chatPresentation.js').includes('export function readableChatError')) failures.push('chat presentation helpers missing');
if (!read('web/src/lib/toolEventDetails.js').includes('export function buildToolEventDetail')) failures.push('tool event detail helpers missing');

const settingsActions = read('web/src/hooks/useSettingsActions.js');
if (!settingsActions.includes('export function useSettingsActions')) failures.push('settings mutations belong in web/src/hooks/useSettingsActions.js');
for (const name of ['editProject', 'editModelProvider', 'editScheduledTask']) {
  if (app.includes('const ' + name + ' = useCallback')) failures.push(`settings action should stay outside App.jsx: ${name}`);
}
for (const [stateName, setterName] of [['selectedScheduledTaskID', 'setSelectedScheduledTaskID'], ['selectedScheduledTaskRuns', 'setSelectedScheduledTaskRuns']]) {
  if (app.includes(setterName + '(') && !app.includes(`const [${stateName}, ${setterName}] = useState`)) failures.push(`${setterName} is used without local state ownership`);
}
const appLineCount = app.split(/\n/).length;
if (appLineCount > 1300) failures.push(`App.jsx line count ${appLineCount} exceeds 1300; keep settings mutations in useSettingsActions.js and shell JSX in appChrome.jsx`);
const appChrome = read('web/src/components/appChrome.jsx');
for (const name of ['Sidebar', 'Topbar', 'ComposerBar']) {
  if (!appChrome.includes('export function ' + name)) failures.push(`app chrome component missing: ${name}`);
  if (app.includes('function ' + name)) failures.push(`app chrome component should stay outside App.jsx: ${name}`);
}
if (!read('web/src/lib/quickActions.js').includes('export function buildQuickActions')) failures.push('quick action construction belongs in web/src/lib/quickActions.js');
const componentExportChecks = [
  ['web/src/components/chat.jsx', 'export function MessageView'],
  ['web/src/components/settings.jsx', 'export function SettingsPanel'],
  ['web/src/components/base.jsx', 'export function DialogHost'],
  ['web/src/hooks/useAttachments.js', 'export function useAttachments'],
  ['web/src/hooks/useSettingsData.js', 'export function useSettingsData'],
  ['web/src/components/managementPages.jsx', 'export function ManagementPage'],
  ['web/src/lib/sessionPresentation.js', 'export function visibleSessionRows'],
];
for (const [file, token] of componentExportChecks) {
  if (!read(file).includes(token)) failures.push(`expected export missing in ${file}: ${token}`);
}


const cssResiduals = [
  '.task-card, .task-card',
  '.task-head, .task-head',
  '.task-name, .task-name',
  '.task-desc, .task-desc',
  '.task-actions, .task-actions',
  '.task-toggle, .task-toggle',
];
for (const file of ['web/src/styles/settings.css', 'web/src/styles/layout.css']) {
  const content = read(file);
  for (const residual of cssResiduals) {
    if (content.includes(residual)) failures.push(`${file} contains duplicated residual selector: ${residual}`);
  }
}

const providerFormSource = read('web/src/lib/modelProviderForm.js');
for (const stale of ['provider.apiKeys', 'key.apiKey', 'key.apiKeyMasked', 'item?.apiKey', 'provider.keyStrategy', 'provider.selectedKeyID']) {
  if (app.includes(stale) || providerFormSource.includes(stale)) failures.push(`model provider frontend contract must use snake_case only: ${stale}`);
}

const providerForm = await import('../web/src/lib/modelProviderForm.js');
const modelNames = providerForm.uniqueModelNames('a\na，b,b');
if (modelNames.join(',') !== 'a,b') failures.push('uniqueModelNames should trim and dedupe comma/newline separated names');
const rows = providerForm.providerKeyRows({ api_keys: [{ id: 'main', name: 'main', has_api_key: true }] });
if (rows[0]?.api_key !== '********' || !rows[0]?.saved) failures.push('providerKeyRows should represent saved keys with a mask');
const inputs = providerForm.providerKeyInputsFromRows([{ id: 'main', name: '主 key', api_key: '********', saved: true }]);
if (inputs?.[0]?.api_key !== '********') failures.push('providerKeyInputsFromRows should preserve masked saved keys');


const packageJSON = JSON.parse(read('web/package.json'));
if (!packageJSON.dependencies?.['lucide-react']) failures.push('lucide-react must remain the single UI icon library');

const walkFiles = (directory, suffixes) => fs.readdirSync(path.join(root, directory), { withFileTypes: true }).flatMap(entry => {
  const relative = path.join(directory, entry.name);
  if (entry.isDirectory()) return walkFiles(relative, suffixes);
  return suffixes.some(suffix => entry.name.endsWith(suffix)) ? [relative] : [];
});
for (const file of walkFiles('web/src', ['.js', '.jsx', '.css'])) {
  if (read(file).toLowerCase().includes('workspace')) {
    failures.push(`${file} still uses the removed workspace concept; use project, task, or management terminology`);
  }
}
const iconComponentFiles = ['web/src/App.jsx', ...walkFiles('web/src/components', ['.jsx'])];
for (const file of iconComponentFiles) {
  if (read(file).includes('<svg')) failures.push(`${file} contains an inline SVG; use a Lucide component instead`);
}
const visibleUISourceFiles = [...iconComponentFiles, ...walkFiles('web/src/styles', ['.css'])];
const emojiPattern = /[\u{1F300}-\u{1FAFF}\u2600-\u27BF]/u;
for (const file of visibleUISourceFiles) {
  if (emojiPattern.test(read(file))) failures.push(`${file} contains an emoji or symbol glyph; use a Lucide icon or CSS geometry instead`);
}

if (failures.length) {
  console.error(failures.map(item => `frontend lint: ${item}`).join('\n'));
  process.exit(1);
}
console.log('frontend lint ok');
