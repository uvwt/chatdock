import test from 'node:test';
import assert from 'node:assert/strict';
import {clampMCPAppHeight, MCP_APP_SANDBOX_PROXY_URL, mcpAppArgumentsFromEvent, mcpAppCSP, mcpAppInitializeResult, mcpAppResultFromEvent, mcpAppSandboxResourceNotification, mcpAppSrcDoc, mcpAppToolInputNotification, mcpAppToolResultNotification} from './mcpApps.js';

test('MCP App CSP defaults to no network and ignores unsafe domain tokens', () => {
  const app = {_meta: {ui: {csp: {
    connectDomains: ['https://api.example.com', 'javascript:alert(1)', 'https://bad.example/a'],
    resourceDomains: ['https://cdn.example.com'],
  }}}};
  const csp = mcpAppCSP(app);
  assert.match(csp, /connect-src https:\/\/api\.example\.com/);
  assert.doesNotMatch(csp, /javascript|bad\.example/);
  assert.match(csp, /object-src 'none'/);
  assert.match(csp, /form-action 'none'/);
});

test('MCP App host prepends its CSP without executing HTML in parent DOM', () => {
  const srcDoc = mcpAppSrcDoc({html: '<!doctype html><html><head><title>x</title></head><body>ok</body></html>'});
  assert.match(srcDoc, /<head><meta http-equiv="Content-Security-Policy"/);
  assert.match(srcDoc, /default-src 'none'/);
});


test('CSP injection preserves doctype when app HTML has no head', () => {
  const srcDoc = mcpAppSrcDoc({html: '<!doctype html><html><body>ok</body></html>'});
  assert.match(srcDoc, /^<!doctype html><html><head><meta http-equiv="Content-Security-Policy"/i);
  const proxyHTML = decodeURIComponent(MCP_APP_SANDBOX_PROXY_URL.split(',', 2)[1]);
  assert.match(proxyHTML, /if\(\/<html/);
  assert.doesNotMatch(srcDoc, /^<meta[^>]+><!doctype/i);
});

test('tool result notification forwards only MCP result fields', () => {
  const notification = mcpAppToolResultNotification({content: [{type: 'text', text: 'ok'}], structuredContent: {ok: true}, isError: false, _meta: {x: 1}, html: '<secret>'});
  assert.equal(notification.method, 'ui/notifications/tool-result');
  assert.deepEqual(notification.params.structuredContent, {ok: true});
  assert.equal(notification.params._meta.x, 1);
  assert.equal('html' in notification.params, false);
});

test('MCP App height is bounded', () => {
  assert.equal(clampMCPAppHeight(5), 48);
  assert.equal(clampMCPAppHeight(333.2), 334);
  assert.equal(clampMCPAppHeight(9000), 1200);
});


test('mcpAppResultFromEvent reads live tool result', () => {
  const result = {content: [{type: 'text', text: 'ok'}], structuredContent: {answer: 42}};
  assert.deepEqual(mcpAppResultFromEvent({details: {data: {result}}}), result);
});

test('mcpAppResultFromEvent supports hydrated direct result', () => {
  const result = {structuredContent: {answer: 'history'}};
  assert.deepEqual(mcpAppResultFromEvent({details: {result, data: {}}}), result);
});


test('MCP App Sandbox Proxy implements the mandatory two-frame handshake', () => {
  assert.match(MCP_APP_SANDBOX_PROXY_URL, /^data:text\/html;charset=utf-8,/);
  const proxyHTML = decodeURIComponent(MCP_APP_SANDBOX_PROXY_URL.split(',', 2)[1]);
  assert.match(proxyHTML, /ui\/notifications\/sandbox-proxy-ready/);
  assert.match(proxyHTML, /ui\/notifications\/sandbox-resource-ready/);
  assert.match(proxyHTML, /document\.createElement\('iframe'\)/);
  assert.match(proxyHTML, /message\.method\.startsWith\('ui\/notifications\/sandbox-'\)/);
  assert.match(proxyHTML, /frame\.srcdoc=injectCSP/);
  assert.match(proxyHTML, /readyAttempts>=20/);
  assert.match(proxyHTML, /readyTimer=setInterval\(announceReady,250\)/);
  assert.match(proxyHTML, /if\(readyTimer\)clearInterval\(readyTimer\)/);
});

test('sandbox resource message carries raw HTML, approved CSP, and declared permissions', () => {
  const notification = mcpAppSandboxResourceNotification({
    html: '<!doctype html><p>view</p>',
    _meta: {ui: {
      csp: {
        connectDomains: ['wss://socket.example.com', 'https://*.api.example.com', 'https://bad.example/path'],
        resourceDomains: ['https://cdn.example.com'],
        baseUriDomains: ['https://base.example.com'],
      },
      permissions: {camera: {}, clipboardWrite: {}, unknown: {}},
    }},
  });
  assert.equal(notification.method, 'ui/notifications/sandbox-resource-ready');
  assert.equal(notification.params.html, '<!doctype html><p>view</p>');
  assert.deepEqual(notification.params.csp.connectDomains, ['wss://socket.example.com', 'https://*.api.example.com']);
  assert.deepEqual(notification.params.csp.baseUriDomains, ['https://base.example.com']);
  assert.deepEqual(notification.params.permissions, {camera: {}, clipboardWrite: {}});
});

test('initialize result advertises only implemented app host capabilities', () => {
  const result = mcpAppInitializeResult({_meta: {ui: {csp: {connectDomains: ['https://api.example.com']}}}}, {serverTools: true});
  assert.equal(result.protocolVersion, '2026-01-26');
  assert.deepEqual(result.hostCapabilities.serverTools, {});
  assert.equal('serverResources' in result.hostCapabilities, false);
  assert.deepEqual(result.hostCapabilities.sandbox.csp.connectDomains, ['https://api.example.com']);
  assert.equal(result.hostContext.displayMode, 'inline');
  assert.deepEqual(result.hostContext.availableDisplayModes, ['inline']);
});

test('tool input precedes result with the complete event arguments shape', () => {
  const args = {path: '/tmp/demo', options: {force: false}};
  assert.deepEqual(mcpAppArgumentsFromEvent({details: {data: {arguments: args}}}), args);
  const input = mcpAppToolInputNotification(args);
  assert.equal(input.method, 'ui/notifications/tool-input');
  assert.deepEqual(input.params.arguments, args);
});
