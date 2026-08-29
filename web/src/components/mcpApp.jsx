import React, { useEffect, useRef, useState } from 'react';
import {
  clampMCPAppHeight,
  MCP_APP_PROTOCOL_VERSION,
  MCP_APP_SANDBOX_PROXY_URL,
  mcpAppInitializeResult,
  mcpAppPrefersBorder,
  mcpAppSandboxResourceNotification,
  mcpAppToolInputNotification,
  mcpAppToolResultNotification,
} from '../lib/mcpApps.js';

function rpcError(id, code, message) {
  return {jsonrpc: '2.0', id, error: {code, message}};
}

function rpcResult(id, result) {
  return {jsonrpc: '2.0', id, result};
}

export function MCPAppFrame({app, arguments: toolArguments, result, sourceTool, onToolCall}) {
  const iframeRef = useRef(null);
  const initializedRef = useRef(false);
  const inputSentRef = useRef(false);
  const [ready, setReady] = useState(false);
  const [height, setHeight] = useState(72);
  const [bridgeError, setBridgeError] = useState('');
  const resourceKey = `${app?.server || ''}:${app?.resource_uri || ''}`;
  const hasHTML = !!String(app?.html || '');

  function post(message) {
    iframeRef.current?.contentWindow?.postMessage(message, '*');
  }

  useEffect(() => {
    setReady(false);
    setHeight(72);
    setBridgeError('');
    initializedRef.current = false;
    inputSentRef.current = false;
  }, [resourceKey]);

  useEffect(() => {
    const handleMessage = async event => {
      const proxyWindow = iframeRef.current?.contentWindow;
      if (!proxyWindow || event.source !== proxyWindow || event.origin !== 'null') return;
      const message = event.data;
      if (!message || typeof message !== 'object' || message.jsonrpc !== '2.0') return;

      if (message.method === 'ui/notifications/sandbox-proxy-ready') {
        proxyWindow.postMessage(mcpAppSandboxResourceNotification(app), '*');
        return;
      }
      if (message.method === 'ui/notifications/size-changed') {
        setHeight(clampMCPAppHeight(message.params?.height));
        return;
      }
      if (message.method === 'ui/notifications/initialized') {
        initializedRef.current = true;
        setReady(true);
        return;
      }
      if (message.id === undefined || !message.method) return;

      if (message.method === 'ui/initialize') {
        proxyWindow.postMessage(rpcResult(message.id, mcpAppInitializeResult(app, {serverTools: !!onToolCall})), '*');
        return;
      }
      if (message.method === 'ping') {
        proxyWindow.postMessage(rpcResult(message.id, {}), '*');
        return;
      }
      if (message.method === 'tools/call') {
        const name = String(message.params?.name || '').trim();
        if (!name || !onToolCall) {
          proxyWindow.postMessage(rpcError(message.id, -32602, 'MCP App tool call is unavailable.'), '*');
          return;
        }
        try {
          const toolResult = await onToolCall({sourceTool, name, arguments: message.params?.arguments || {}});
          proxyWindow.postMessage(rpcResult(message.id, toolResult), '*');
        } catch (error) {
          proxyWindow.postMessage(rpcError(message.id, -32000, String(error?.message || error || 'MCP App tool call failed.')), '*');
        }
        return;
      }
      proxyWindow.postMessage(rpcError(message.id, -32601, `Unsupported MCP App method: ${message.method}`), '*');
    };
    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [app, onToolCall, sourceTool]);

  useEffect(() => {
    if (!ready || !iframeRef.current?.contentWindow) return;
    if (!inputSentRef.current) {
      post(mcpAppToolInputNotification(toolArguments));
      inputSentRef.current = true;
    }
    post(mcpAppToolResultNotification(result));
  }, [ready, result, toolArguments]);

  useEffect(() => () => {
    if (!initializedRef.current || !iframeRef.current?.contentWindow) return;
    iframeRef.current.contentWindow.postMessage({
      jsonrpc: '2.0',
      id: `teardown-${Date.now()}`,
      method: 'ui/resource-teardown',
      params: {reason: 'view removed'},
    }, '*');
  }, [resourceKey]);

  if (!hasHTML) return null;
  return <div className={'mcp-app-host ' + (mcpAppPrefersBorder(app) ? 'with-border' : 'borderless')}>
    <iframe
      key={resourceKey}
      ref={iframeRef}
      className="mcp-app-frame"
      title={`MCP App · ${app?.server || 'server'} · ${app?.resource_uri || 'resource'}`}
      sandbox="allow-scripts allow-same-origin"
      referrerPolicy="no-referrer"
      src={MCP_APP_SANDBOX_PROXY_URL}
      style={{height: `${height}px`}}
      onError={() => setBridgeError('MCP App 加载失败')}
    />
    {bridgeError ? <div className="mcp-app-error" role="status">{bridgeError}</div> : null}
    <span className="mcp-app-protocol" aria-hidden="true">MCP Apps {MCP_APP_PROTOCOL_VERSION}</span>
  </div>;
}
