export function renderMarkdown(value) {
  const lines = String(value || '').replace(/\r\n/g, '\n').split('\n');
  const html = [];
  let paragraph = [];
  let listType = null;
  let listItems = [];
  let inCode = false;
  let codeLines = [];
  let codeLang = '';

  function flushParagraph() {
    if (!paragraph.length) return;
    html.push('<p>' + renderInline(paragraph.join(' ')) + '</p>');
    paragraph = [];
  }
  function flushList() {
    if (!listType) return;
    html.push('<' + listType + '>' + listItems.map(item => '<li>' + renderInline(item) + '</li>').join('') + '</' + listType + '>');
    listType = null;
    listItems = [];
  }
  function flushCode() {
    const langClass = codeLang ? ' class="language-' + escapeHtml(codeLang) + '"' : '';
    html.push('<pre><code' + langClass + '>' + escapeHtml(codeLines.join('\n')) + '</code></pre>');
    inCode = false;
    codeLines = [];
    codeLang = '';
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const fence = line.match(/^\s*```\s*([^`]*)\s*$/);
    if (fence) {
      if (inCode) flushCode();
      else {
        flushParagraph();
        flushList();
        inCode = true;
        codeLang = fence[1].trim().replace(/[^\w#+.-]/g, '');
      }
      continue;
    }
    if (inCode) {
      codeLines.push(line);
      continue;
    }
    if (isMarkdownTable(lines, i)) {
      flushParagraph();
      flushList();
      const table = readMarkdownTable(lines, i);
      html.push(renderMarkdownTable(table.rows, table.alignments));
      i = table.nextIndex - 1;
      continue;
    }
    if (!line.trim()) {
      flushParagraph();
      flushList();
      continue;
    }
    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      flushParagraph();
      flushList();
      const level = heading[1].length;
      html.push('<h' + level + '>' + renderInline(heading[2].trim()) + '</h' + level + '>');
      continue;
    }
    const quote = line.match(/^>\s?(.+)$/);
    if (quote) {
      flushParagraph();
      flushList();
      html.push('<blockquote>' + renderInline(quote[1].trim()) + '</blockquote>');
      continue;
    }
    const unordered = line.match(/^\s*[-*+]\s+(.+)$/);
    const ordered = line.match(/^\s*\d+[.)]\s+(.+)$/);
    if (unordered || ordered) {
      flushParagraph();
      const nextType = unordered ? 'ul' : 'ol';
      if (listType && listType !== nextType) flushList();
      listType = nextType;
      listItems.push((unordered || ordered)[1]);
      continue;
    }
    paragraph.push(line.trim());
  }
  if (inCode) flushCode();
  flushParagraph();
  flushList();
  return html.join('') || '';
}

function isMarkdownTable(lines, index) {
  if (index + 1 >= lines.length) return false;
  const header = splitMarkdownTableRow(lines[index]);
  const separator = splitMarkdownTableRow(lines[index + 1]);
  if (header.length < 2 || separator.length < 2 || header.length !== separator.length) return false;
  return separator.every(cell => /^:?-{3,}:?$/.test(cell.trim()));
}

function readMarkdownTable(lines, index) {
  const rows = [splitMarkdownTableRow(lines[index])];
  const separator = splitMarkdownTableRow(lines[index + 1]);
  const alignments = separator.map(cell => {
    cell = cell.trim();
    if (cell.startsWith(':') && cell.endsWith(':')) return 'center';
    if (cell.endsWith(':')) return 'right';
    return 'left';
  });
  let nextIndex = index + 2;
  while (nextIndex < lines.length) {
    const row = splitMarkdownTableRow(lines[nextIndex]);
    if (row.length !== rows[0].length) break;
    rows.push(row);
    nextIndex++;
  }
  return {rows, alignments, nextIndex};
}

function splitMarkdownTableRow(line) {
  const trimmed = String(line || '').trim();
  if (!trimmed.includes('|')) return [];
  const body = trimmed.replace(/^\|/, '').replace(/\|$/, '');
  const cells = [];
  let current = '';
  let escaped = false;
  for (const ch of body) {
    if (escaped) {
      current += ch;
      escaped = false;
      continue;
    }
    if (ch === '\\') {
      escaped = true;
      continue;
    }
    if (ch === '|') {
      cells.push(current.trim());
      current = '';
      continue;
    }
    current += ch;
  }
  cells.push(current.trim());
  return cells;
}

function renderMarkdownTable(rows, alignments) {
  const head = rows[0] || [];
  const body = rows.slice(1);
  const ths = head.map((cell, i) => '<th style="text-align:' + alignments[i] + '">' + renderInline(cell) + '</th>').join('');
  const trs = body.map(row => '<tr>' + row.map((cell, i) => '<td style="text-align:' + alignments[i] + '">' + renderInline(cell) + '</td>').join('') + '</tr>').join('');
  return '<div class="table-wrap"><table><thead><tr>' + ths + '</tr></thead><tbody>' + trs + '</tbody></table></div>';
}

function renderInline(value) {
  let html = escapeHtml(value);
  const codes = [];
  html = html.replace(/`([^`]+)`/g, (_, code) => {
    const key = '\u0000CODE' + codes.length + '\u0000';
    codes.push('<code>' + code + '</code>');
    return key;
  });
  html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+|\/[^\s)]+|#[^\s)]+)\)/g, (_, text, url) => {
    return '<a href="' + url + '" target="_blank" rel="noopener noreferrer">' + text + '</a>';
  });
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/(^|\s)\*([^*]+)\*/g, '$1<em>$2</em>');
  html = html.replace(/(^|\s)_([^_]+)_/g, '$1<em>$2</em>');
  for (let i = 0; i < codes.length; i++) {
    html = html.replace('\u0000CODE' + i + '\u0000', codes[i]);
  }
  return html;
}

export function escapeHtml(value) {
  return String(value || '').replace(/[&<>\"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','\"':'&quot;'}[c]));
}
