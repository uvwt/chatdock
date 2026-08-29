export const MCP_APP_PROTOCOL_VERSION = '2026-01-26';

const MAX_APP_HEIGHT = 1200;
const MIN_APP_HEIGHT = 48;

function uiMeta(app) {
  const ui = app?._meta?.ui;
  return ui && typeof ui === 'object' && !Array.isArray(ui) ? ui : {};
}

function normalizeCSPSource(value, allowedProtocols) {
  const raw = String(value || '').trim();
  if (!raw || /[\s;'"\\]/.test(raw)) return '';

  const wildcard = raw.match(/^(https?|wss?):\/\/\*\.([a-z0-9-]+(?:\.[a-z0-9-]+)+)(?::([1-9]\d{0,4}))?$/i);
  if (wildcard) {
    const protocol = wildcard[1].toLowerCase() + ':';
    const port = wildcard[3] ? Number(wildcard[3]) : 0;
    if (!allowedProtocols.includes(protocol) || port > 65535) return '';
    return raw.toLowerCase();
  }

  try {
    const url = new URL(raw);
    if (!allowedProtocols.includes(url.protocol)) return '';
    if (url.username || url.password || url.pathname !== '/' || url.search || url.hash) return '';
    return url.origin;
  } catch {
    return '';
  }
}

function allowedSources(values, allowedProtocols) {
  if (!Array.isArray(values)) return [];
  return [...new Set(values.map(value => normalizeCSPSource(value, allowedProtocols)).filter(Boolean))];
}

function sourceList(values, allowedProtocols, defaults = []) {
  const sources = [...defaults, ...allowedSources(values, allowedProtocols)];
  return sources.length ? sources.join(' ') : "'none'";
}

export function mcpAppApprovedCSP(app) {
  const csp = uiMeta(app).csp || {};
  return {
    connectDomains: allowedSources(csp.connectDomains, ['http:', 'https:', 'ws:', 'wss:']),
    resourceDomains: allowedSources(csp.resourceDomains, ['http:', 'https:']),
    frameDomains: allowedSources(csp.frameDomains, ['http:', 'https:']),
    baseUriDomains: allowedSources(csp.baseUriDomains, ['http:', 'https:']),
  };
}

export function mcpAppCSP(app) {
  const csp = mcpAppApprovedCSP(app);
  const resources = csp.resourceDomains;
  return [
    "default-src 'none'",
    `script-src ${sourceList(resources, ['http:', 'https:'], ["'self'", "'unsafe-inline'"])}`,
    `style-src ${sourceList(resources, ['http:', 'https:'], ["'self'", "'unsafe-inline'"])}`,
    `img-src ${sourceList(resources, ['http:', 'https:'], ["'self'", 'data:'])}`,
    `font-src ${sourceList(resources, ['http:', 'https:'], ["'self'"])}`,
    `media-src ${sourceList(resources, ['http:', 'https:'], ["'self'", 'data:'])}`,
    `connect-src ${sourceList(csp.connectDomains, ['http:', 'https:', 'ws:', 'wss:'])}`,
    `frame-src ${sourceList(csp.frameDomains, ['http:', 'https:'])}`,
    `base-uri ${sourceList(csp.baseUriDomains, ['http:', 'https:'], ["'self'"])}`,
    "worker-src 'none'",
    "object-src 'none'",
    "form-action 'none'",
  ].join('; ');
}

function escapeHTMLAttribute(value) {
  return String(value).replaceAll('&', '&amp;').replaceAll('"', '&quot;');
}

function injectCSPMeta(html, meta) {
  if (/<head(?:\s[^>]*)?>/i.test(html)) return html.replace(/<head(\s[^>]*)?>/i, match => match + meta);
  if (/<html(?:\s[^>]*)?>/i.test(html)) return html.replace(/<html(\s[^>]*)?>/i, match => match + `<head>${meta}</head>`);
  if (/^\s*<!doctype[^>]*>/i.test(html)) return html.replace(/^(\s*<!doctype[^>]*>)/i, `$1${meta}`);
  return meta + html;
}

export function mcpAppSrcDoc(app) {
  const html = String(app?.html || '');
  if (!html) return '';
  const meta = `<meta http-equiv="Content-Security-Policy" content="${escapeHTMLAttribute(mcpAppCSP(app))}">`;
  return injectCSPMeta(html, meta);
}

function permissionConfig(app) {
  const requested = uiMeta(app).permissions;
  if (!requested || typeof requested !== 'object' || Array.isArray(requested)) return {};
  const permissions = {};
  for (const key of ['camera', 'microphone', 'geolocation', 'clipboardWrite']) {
    if (requested[key] && typeof requested[key] === 'object') permissions[key] = {};
  }
  return permissions;
}

export function mcpAppSandboxResourceNotification(app) {
  return {
    jsonrpc: '2.0',
    method: 'ui/notifications/sandbox-resource-ready',
    params: {
      html: String(app?.html || ''),
      sandbox: 'allow-scripts',
      csp: mcpAppApprovedCSP(app),
      permissions: permissionConfig(app),
    },
  };
}

export function mcpAppArgumentsFromEvent(event) {
  const details = event?.details;
  if (!details || typeof details !== 'object') return {};
  const data = details.data && typeof details.data === 'object' ? details.data : {};
  const args = details.arguments ?? data.arguments;
  return args && typeof args === 'object' && !Array.isArray(args) ? args : {};
}

export function mcpAppResultFromEvent(event) {
  const details = event?.details;
  if (!details || typeof details !== 'object') return {};
  const data = details.data && typeof details.data === 'object' ? details.data : {};
  const result = details.result ?? data.result;
  return result && typeof result === 'object' ? result : {};
}

export function mcpAppToolInputNotification(argumentsValue) {
  const args = argumentsValue && typeof argumentsValue === 'object' && !Array.isArray(argumentsValue) ? argumentsValue : {};
  return {jsonrpc: '2.0', method: 'ui/notifications/tool-input', params: {arguments: args}};
}

export function mcpAppToolResultNotification(result) {
  const normalized = result && typeof result === 'object' ? result : {};
  return {
    jsonrpc: '2.0',
    method: 'ui/notifications/tool-result',
    params: {
      content: Array.isArray(normalized.content) ? normalized.content : [],
      structuredContent: normalized.structuredContent ?? null,
      isError: !!normalized.isError,
      _meta: normalized._meta && typeof normalized._meta === 'object' ? normalized._meta : {},
    },
  };
}

export function mcpAppInitializeResult(app, {serverTools = true} = {}) {
  const sandboxCSP = mcpAppApprovedCSP(app);
  return {
    protocolVersion: MCP_APP_PROTOCOL_VERSION,
    hostInfo: {name: 'chatdock', version: '1.0.0'},
    hostCapabilities: {
      ...(serverTools ? {serverTools: {}} : {}),
      sandbox: {csp: sandboxCSP},
    },
    hostContext: {
      displayMode: 'inline',
      availableDisplayModes: ['inline'],
      containerDimensions: {maxHeight: MAX_APP_HEIGHT},
      userAgent: 'chatdock',
      platform: 'web',
    },
  };
}

export function clampMCPAppHeight(value) {
  const height = Number(value);
  if (!Number.isFinite(height)) return MIN_APP_HEIGHT;
  return Math.min(MAX_APP_HEIGHT, Math.max(MIN_APP_HEIGHT, Math.ceil(height)));
}

export function mcpAppPrefersBorder(app) {
  return uiMeta(app).prefersBorder !== false;
}

const SANDBOX_PROXY_HTML = String.raw`<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;padding:0;width:100%;height:100%;overflow:hidden}iframe{display:block;width:100%;height:100%;border:0;background:transparent}</style></head><body><script>
(()=>{
  const host=window.parent;
  let view=null;
  let resourceReady=false;
  let readyAttempts=0;
  let readyTimer=null;
  const allowedProtocols={connect:['http:','https:','ws:','wss:'],resource:['http:','https:'],frame:['http:','https:'],base:['http:','https:']};
  function validSource(value,protocols){
    const raw=String(value||'').trim();
    if(!raw||/[\s;'"\\]/.test(raw))return '';
    const wildcard=raw.match(/^(https?|wss?):\/\/\*\.([a-z0-9-]+(?:\.[a-z0-9-]+)+)(?::([1-9]\d{0,4}))?$/i);
    if(wildcard){const protocol=wildcard[1].toLowerCase()+':',port=wildcard[3]?Number(wildcard[3]):0;return protocols.includes(protocol)&&port<=65535?raw.toLowerCase():'';}
    try{const url=new URL(raw);return protocols.includes(url.protocol)&&!url.username&&!url.password&&url.pathname==='/'&&!url.search&&!url.hash?url.origin:'';}catch{return '';}
  }
  function sources(values,protocols,defaults=[]){
    const declared=Array.isArray(values)?values.map(value=>validSource(value,protocols)).filter(Boolean):[];
    const unique=[...new Set([...defaults,...declared])];
    return unique.length?unique.join(' '):"'none'";
  }
  function cspText(csp={}){
    const resources=csp.resourceDomains;
    return [
      "default-src 'none'",
      "script-src "+sources(resources,allowedProtocols.resource,["'self'","'unsafe-inline'"]),
      "style-src "+sources(resources,allowedProtocols.resource,["'self'","'unsafe-inline'"]),
      "img-src "+sources(resources,allowedProtocols.resource,["'self'","data:"]),
      "font-src "+sources(resources,allowedProtocols.resource,["'self'"]),
      "media-src "+sources(resources,allowedProtocols.resource,["'self'","data:"]),
      "connect-src "+sources(csp.connectDomains,allowedProtocols.connect),
      "frame-src "+sources(csp.frameDomains,allowedProtocols.frame),
      "base-uri "+sources(csp.baseUriDomains,allowedProtocols.base,["'self'"]),
      "worker-src 'none'","object-src 'none'","form-action 'none'"
    ].join('; ');
  }
  function injectCSP(html,csp){
    const escaped=cspText(csp).replaceAll('&','&amp;').replaceAll('"','&quot;');
    const meta='<meta http-equiv="Content-Security-Policy" content="'+escaped+'">';
    if(/<head(?:\s[^>]*)?>/i.test(html))return html.replace(/<head(\s[^>]*)?>/i,match=>match+meta);
    if(/<html(?:\s[^>]*)?>/i.test(html))return html.replace(/<html(\s[^>]*)?>/i,match=>match+'<head>'+meta+'</head>');
    if(/^\s*<!doctype[^>]*>/i.test(html))return html.replace(/^(\s*<!doctype[^>]*>)/i,'$1'+meta);
    return meta+html;
  }
  function allowValue(permissions={}){
    const names=[];
    if(permissions.camera)names.push('camera');
    if(permissions.microphone)names.push('microphone');
    if(permissions.geolocation)names.push('geolocation');
    if(permissions.clipboardWrite)names.push('clipboard-write');
    return names.join('; ');
  }
  function forwardable(message){return message&&typeof message==='object'&&message.jsonrpc==='2.0'&&!(typeof message.method==='string'&&message.method.startsWith('ui/notifications/sandbox-'));}
  function loadResource(params={}){
    if(view){view.remove();view=null;}
    const frame=document.createElement('iframe');
    frame.setAttribute('sandbox',String(params.sandbox||'allow-scripts'));
    frame.referrerPolicy='no-referrer';
    const allow=allowValue(params.permissions);
    if(allow)frame.setAttribute('allow',allow);
    document.body.replaceChildren(frame);
    view=frame;
    frame.srcdoc=injectCSP(String(params.html||''),params.csp||{});
  }
  window.addEventListener('message',event=>{
    const message=event.data;
    if(event.source===host){
      if(message&&message.jsonrpc==='2.0'&&message.method==='ui/notifications/sandbox-resource-ready'){resourceReady=true;if(readyTimer)clearInterval(readyTimer);loadResource(message.params||{});return;}
      if(view&&view.contentWindow&&forwardable(message))view.contentWindow.postMessage(message,'*');
      return;
    }
    if(view&&event.source===view.contentWindow&&forwardable(message))host.postMessage(message,'*');
  });
  function announceReady(){
    if(resourceReady||readyAttempts>=20){if(readyTimer)clearInterval(readyTimer);return;}
    readyAttempts++;
    host.postMessage({jsonrpc:'2.0',method:'ui/notifications/sandbox-proxy-ready',params:{}},'*');
  }
  announceReady();
  readyTimer=setInterval(announceReady,250);
})();
</script></body></html>`;

export const MCP_APP_SANDBOX_PROXY_URL = `data:text/html;charset=utf-8,${encodeURIComponent(SANDBOX_PROXY_HTML)}`;
