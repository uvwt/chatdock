// Markdown rendering is isolated so chat and settings shells do not load parser dependencies until content needs them.
import React, { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

function normalizeCodeLanguage(value = '') {
  const raw = String(value || '').replace(/^language-/, '').trim().toLowerCase();
  const aliases = {
    js: 'javascript', jsx: 'jsx', ts: 'typescript', tsx: 'tsx', py: 'python', rb: 'ruby', sh: 'bash', shell: 'bash', zsh: 'bash',
    yml: 'yaml', md: 'markdown', html: 'html', xml: 'xml', golang: 'go', dockerfile: 'dockerfile', jsonc: 'json',
  };
  return aliases[raw] || raw || 'text';
}

const codeKeywordRE = /^(?:const|let|var|function|return|if|else|for|while|do|switch|case|default|break|continue|class|extends|new|import|from|export|async|await|try|catch|finally|throw|true|false|null|undefined|def|self|None|True|False|elif|lambda|with|as|pass|yield|package|func|struct|type|interface|map|range|go|defer|select|chan|public|private|protected|static|final|void|int|string|bool|float|double|echo|cd|grep|curl|docker|git|npm|pnpm|yarn|make|sudo)$/;

function codeTokenClass(token) {
  if (!token) return '';
  if (/^(?:\/\/.*|#.*|\/\*.*\*\/)$/s.test(token)) return 'hl-comment';
  if (/^(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`)$/s.test(token)) return 'hl-string';
  if (/^\d+(?:\.\d+)?$/.test(token)) return 'hl-number';
  if (codeKeywordRE.test(token)) return 'hl-keyword';
  if (/^[{}()[\].,;:+\-*/%=<>!&|]+$/.test(token)) return 'hl-punctuation';
  return '';
}

function highlightCode(code) {
  const tokenRE = /(\/\/[^\n]*|#[^\n]*|\/\*.*?\*\/|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|\b\d+(?:\.\d+)?\b|\b[A-Za-z_$][\w$-]*\b|[{}()[\].,;:+\-*/%=<>!&|]+)/gs;
  const nodes = [];
  let cursor = 0;
  let index = 0;
  for (const match of code.matchAll(tokenRE)) {
    if (match.index > cursor) nodes.push(code.slice(cursor, match.index));
    const token = match[0];
    const cls = codeTokenClass(token);
    nodes.push(cls ? <span key={index++} className={cls}>{token}</span> : token);
    cursor = match.index + token.length;
  }
  if (cursor < code.length) nodes.push(code.slice(cursor));
  return nodes;
}

function copyCodeToClipboard(text, setCopied) {
  const done = () => {
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  };
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(() => fallbackCopyCode(text, done));
    return;
  }
  fallbackCopyCode(text, done);
}

function fallbackCopyCode(text, done) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  document.body.appendChild(textarea);
  textarea.select();
  try { document.execCommand('copy'); done(); }
  finally { document.body.removeChild(textarea); }
}

function CodeBlock({ code, language }) {
  const [copied, setCopied] = useState(false);
  const lang = normalizeCodeLanguage(language);
  return <div className="code-block">
    <div className="code-block-head">
      <span className="code-block-lang">{lang === 'text' ? 'code' : lang}</span>
      <button type="button" className={"code-block-copy " + (copied ? 'copied' : '')} onClick={() => copyCodeToClipboard(code, setCopied)}>{copied ? '已复制' : '复制'}</button>
    </div>
    <pre className={"code-block-pre language-" + lang}><code>{highlightCode(code)}</code></pre>
  </div>;
}

function MarkdownImage({ src, alt = '', ...props }) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!open) return undefined;
    const closeOnEscape = event => { if (event.key === 'Escape') setOpen(false); };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [open]);

  const openPreview = event => {
    if (event.type === 'keydown' && event.key !== 'Enter' && event.key !== ' ') return;
    event.preventDefault();
    setOpen(true);
  };

  const preview = open && typeof document !== 'undefined' ? createPortal(
    <div className="image-preview-backdrop" role="dialog" aria-modal="true" aria-label={alt || '图片预览'} onClick={() => setOpen(false)}>
      <img className="image-preview-content" src={src} alt={alt} onClick={event => event.stopPropagation()} />
    </div>,
    document.body,
  ) : null;

  return <>
    <img {...props} className={[props.className, 'markdown-image'].filter(Boolean).join(' ')} src={src} alt={alt} role="button" tabIndex={0} onClick={openPreview} onKeyDown={openPreview} />
    {preview}
  </>;
}

const markdownComponents = {
  table({node, ...props}) {
    return <div className="table-wrap"><table {...props} /></div>;
  },
  pre({node, children}) {
    const child = React.Children.toArray(children).find(item => React.isValidElement(item) && item.type === 'code');
    const codeNode = node?.children?.find(item => item?.tagName === 'code');
    const astClassName = codeNode?.properties?.className;
    const className = String(child?.props?.className || (Array.isArray(astClassName) ? astClassName.join(' ') : astClassName || ''));
    const language = className.match(/language-([^\s]+)/)?.[1] || '';
    const astCode = codeNode?.children?.map(item => item?.value || '').join('') || '';
    const code = String(child?.props?.children || astCode).replace(/\n$/, '');
    return <CodeBlock code={code} language={language} />;
  },
  code({node, className, children, ...props}) {
    return <code className={className || ''} {...props}>{children}</code>;
  },
  a({node, ...props}) {
    const href = String(props.href || '');
    const external = /^https?:\/\//i.test(href);
    return <a {...props} target={external ? '_blank' : undefined} rel={external ? 'noopener noreferrer' : undefined} />;
  },
  img({node, ...props}) {
    return <MarkdownImage {...props} />;
  },
};

export default function MarkdownRenderer({ value, className = '' }) {
  const content = <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>{value || ''}</ReactMarkdown>;
  return className ? <div className={className}>{content}</div> : content;
}
