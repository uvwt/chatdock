#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const failures = [];
const read = file => fs.readFileSync(path.join(root, file), 'utf8');

const appCss = read('web/src/app.css').trim().split(/\n/).filter(Boolean);
const nonImportLines = appCss.filter(line => !line.startsWith('@import'));
if (nonImportLines.length) failures.push('web/src/app.css must stay import-only; move real rules into web/src/styles/*.css');

const settings = read('web/src/components/settings.jsx');
for (const stale of ['function ReplyModule', 'function EmbeddingModule']) {
  if (settings.includes(stale)) failures.push(`stale unused component remains: ${stale}`);
}

const tokenParser = settings.match(/JSON\.parse\(atob\(parts\[(\d+)\]/);
if (!tokenParser || tokenParser[1] !== '1') failures.push('JWT expiry parser must decode payload parts[1], not header parts[0]');

const app = read('web/src/App.jsx');
if (app.includes('function toolEventText') || app.includes('function mergeToolResultEvent')) failures.push('tool event protocol helpers belong in web/src/lib/toolEvents.js, not App.jsx');

if (failures.length) {
  console.error(failures.map(item => `frontend lint: ${item}`).join('\n'));
  process.exit(1);
}
console.log('frontend lint ok');
