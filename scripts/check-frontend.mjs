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
const settingFetchNames = ['fetchConfig', 'fetchDataStatus', 'fetchMCPConfig', 'fetchMCPStatus', 'fetchModelProviders', 'fetchPrompts', 'fetchScheduledTasks', 'fetchSetupStatus', 'fetchSystemStatus', 'fetchWorkspaces'];
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

const cssResiduals = [
  '.task-card, .task-card',
  '.task-head, .task-head',
  '.task-name, .task-name',
  '.task-desc, .task-desc',
  '.task-actions, .task-actions',
  '.task-toggle, .task-toggle',
];
for (const file of ['web/src/styles/settings.css', 'web/src/styles/layout.css', 'web/src/styles/legacy-overrides.css']) {
  const content = read(file);
  for (const residual of cssResiduals) {
    if (content.includes(residual)) failures.push(`${file} contains duplicated residual selector: ${residual}`);
  }
}

const providerForm = await import('../web/src/lib/modelProviderForm.js');
const modelNames = providerForm.uniqueModelNames('a\na，b,b');
if (modelNames.join(',') !== 'a,b') failures.push('uniqueModelNames should trim and dedupe comma/newline separated names');
const rows = providerForm.providerKeyRows({ api_keys: [{ id: 'main', name: 'main', has_api_key: true }] });
if (rows[0]?.api_key !== '********' || !rows[0]?.saved) failures.push('providerKeyRows should represent saved keys with a mask');
const inputs = providerForm.providerKeyInputsFromRows([{ id: 'main', name: '主 key', api_key: '********', saved: true }]);
if (inputs?.[0]?.api_key !== '********') failures.push('providerKeyInputsFromRows should preserve masked saved keys');

if (failures.length) {
  console.error(failures.map(item => `frontend lint: ${item}`).join('\n'));
  process.exit(1);
}
console.log('frontend lint ok');
