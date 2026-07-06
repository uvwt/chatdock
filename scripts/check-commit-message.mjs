#!/usr/bin/env node
import { execFileSync } from 'node:child_process';

const allowedTypes = ['feat', 'fix', 'refactor', 'perf', 'test', 'docs', 'style', 'build', 'ci', 'chore', 'revert'];
const typePattern = new RegExp('^(' + allowedTypes.join('|') + ')\\([a-z0-9._-]+\\): .+$');
const cjkPattern = /[\u3400-\u9fff]/;

function validate(subject) {
  subject = String(subject || '').trim();
  if (!subject) return 'empty commit subject';
  if (subject.length > 88) return `commit subject is too long (${subject.length}/88): ${subject}`;
  if (!typePattern.test(subject) || !cjkPattern.test(subject)) {
    return `invalid commit subject: ${subject}\nexpected: type(scope): 中文说明\nallowed types: ${allowedTypes.join(', ')}`;
  }
  return '';
}

function gitLogSubjects(range) {
  const args = ['log', '--format=%s'];
  if (range) args.push(range);
  return execFileSync('git', args, {encoding: 'utf8'}).split('\n').map(item => item.trim()).filter(Boolean);
}

const args = process.argv.slice(2);
let subjects = [];
const messageIndex = args.indexOf('--message');
const rangeIndex = args.indexOf('--range');
if (messageIndex >= 0) subjects = [args[messageIndex + 1] || ''];
else if (rangeIndex >= 0) subjects = gitLogSubjects(args[rangeIndex + 1] || '');
else subjects = gitLogSubjects('HEAD~1..HEAD');

const errors = subjects.map(subject => validate(subject)).filter(Boolean);
if (errors.length) {
  console.error(errors.join('\n'));
  process.exit(1);
}
console.log(`commit message check ok: ${subjects.length} subject(s)`);
