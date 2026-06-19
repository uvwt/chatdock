export const pageHtml = `
<div id="sidebarMask" class="sidebar-mask" onclick="closeSidebarOnMobile()"></div>
<div id="settingsMask" class="settings-mask" onclick="closeSettingsPanel()"></div>
<div class="app" id="app">
  <aside>
    <div class="sidebar-head">
      <div class="brand"><span class="brand-text">ChatDock</span><div class="sub">本地优先 AI 工作台</div></div>
      <button id="sidebarToggle" class="sidebar-toggle" onclick="toggleSidebar()" title="折叠侧栏" aria-label="折叠侧栏">‹</button>
    </div>
    <div class="prompt-box">
      <label>工作空间</label>
      <div class="prompt-row">
        <select id="promptSelector" onchange="selectPrompt(this.value)"></select>
        <button class="prompt-add" onclick="createPromptSpace()" title="新建提示词空间">+</button>
      </div>
    </div>
    <input id="sessionSearch" class="session-search" placeholder="搜索会话" oninput="loadSessions()" />
    <button class="new" onclick="newSession()" title="新会话">+ <span class="new-label">新会话</span></button>
    <div id="sessions"></div>
  </aside>

  <main>
    <div class="topbar">
      <div class="top-left">
        <button class="mobile-menu" onclick="toggleSidebar()" title="会话列表" aria-label="会话列表">☰</button>
        <div id="title">未选择会话</div>
      </div>
      <div class="top-actions">
        <button class="secondary config-toggle" onclick="toggleSettingsPanel()" title="配置中心">配置</button>
        <button id="themeToggle" class="theme-toggle" onclick="toggleTheme()" title="切换白天/夜晚">夜晚</button>
        <button class="secondary" onclick="renameCurrent()">重命名</button>
        <button class="secondary" onclick="exportCurrent()">导出</button>
        <button class="danger" onclick="deleteCurrent()">删除</button>
      </div>
    </div>
    <div id="messages" class="messages"><div class="empty">创建一个会话，然后开始聊天。</div></div>
    <div class="composer">
      <textarea id="input" placeholder="输入消息。Enter 发送，Shift+Enter 换行。"></textarea>
      <button id="continueBtn" class="secondary quick-control" onclick="sendQuickMessage('继续')" title="自动发送继续">继续</button>
      <button id="pauseStream" class="secondary stream-control" onclick="toggleStreamPause()" hidden disabled>暂停</button>
      <button id="stopStream" class="danger stream-control" onclick="stopStreaming()" hidden disabled>中断</button>
      <button id="send" onclick="sendMsg()">发送</button>
    </div>
  </main>

  <section class="settings product-panel">
    <div class="settings-header">
      <div><h2>配置中心</h2><div class="hint">模型、工作空间、技能、工具和自动化统一管理。</div></div>
      <div class="settings-header-actions"><button class="secondary small" onclick="returnToChat()">返回对话</button><button class="secondary small" onclick="refreshProductState()">刷新</button></div>
    </div>
    <div id="setupBanner" class="setup-banner"></div>
    <div class="module-tabs" role="tablist">
      <button class="module-tab active" data-module="workspace" onclick="switchSettingsModule('workspace')">工作空间</button>
      <button class="module-tab" data-module="model" onclick="switchSettingsModule('model')">模型</button>
      <button class="module-tab" data-module="skills" onclick="switchSettingsModule('skills')">技能库</button>
      <button class="module-tab" data-module="tools" onclick="switchSettingsModule('tools')">工具中心</button>
      <button class="module-tab" data-module="automation" onclick="switchSettingsModule('automation')">自动化</button>
      <button class="module-tab" data-module="data" onclick="switchSettingsModule('data')">数据</button>
      <button class="module-tab" data-module="security" onclick="switchSettingsModule('security')">安全</button>
    </div>

    <div class="module-view active" data-module-view="workspace">
      <div class="settings-block-head"><label>工作空间概览</label><button class="secondary small" onclick="createPromptSpace()">新增工作空间</button></div>
      <div id="workspaceCards" class="product-list"><div class="hint">正在加载工作空间...</div></div>
      <label>助手设定 / System Prompt</label>
      <textarea id="system_prompt" class="system-prompt-editor"></textarea>
      <div class="row">
        <div><label>上下文消息数</label><input id="max_context_messages" type="number" min="1" max="100" /></div>
        <div><label>Temperature</label><input id="temperature" type="number" min="0" max="2" step="0.1" /></div>
      </div>
      <label class="check-row"><input id="enable_thinking" type="checkbox" /> 启用模型思考</label>
      <label class="check-row"><input id="hide_thinking" type="checkbox" /> 自动隐藏 &lt;think&gt; 思考内容</label>
      <div class="settings-actions">
        <button onclick="saveConfig()">保存工作空间配置</button>
        <button class="secondary" onclick="showPromptPreview()">查看最终 Prompt</button>
      </div>
      <pre id="promptPreview" class="code-preview" hidden></pre>
    </div>

    <div class="module-view" data-module-view="model">
      <div class="hint">兼容 OpenAI Chat Completions 的模型供应商。当前版本保持旧配置兼容，每个工作空间可独立配置模型。</div>
      <label>Base URL</label>
      <input id="base_url" placeholder="https://api.openai.com/v1" />
      <label>API Key</label>
      <input id="api_key" placeholder="留空表示不修改" type="password" />
      <label>Model</label>
      <input id="model" placeholder="gpt-4o-mini" />
      <div class="settings-actions">
        <button onclick="saveConfig()">保存模型设置</button>
        <button class="secondary" onclick="testModelProvider()">测试连接</button>
      </div>
      <div id="providerCards" class="product-list"><div class="hint">正在加载模型供应商...</div></div>
    </div>

    <div class="module-view" data-module-view="skills">
      <div class="settings-block-head"><label>技能库（当前工作空间）</label><button class="secondary small" onclick="editSkill()">新增技能</button></div>
      <input id="skillSearch" class="session-search" placeholder="搜索技能" oninput="renderSkills()" />
      <div id="skills" class="skills-list"><div class="hint">正在加载技能...</div></div>
    </div>

    <div class="module-view" data-module-view="tools">
      <div class="settings-block-head"><label>MCP 工具中心</label><button class="secondary small" onclick="loadMCPStatus()">检测状态</button></div>
      <div id="mcpStatusCards" class="product-list"><div class="hint">尚未检测 MCP 状态。</div></div>
      <label>MCP 配置 JSON</label>
      <textarea id="mcp_config" class="mcp-editor" spellcheck="false" placeholder='{"servers": {}}'></textarea>
      <div class="settings-actions">
        <button class="secondary" onclick="saveMCPConfig()">保存 MCP 配置</button>
        <button class="secondary" onclick="loadMCPConfig()">重新加载 MCP</button>
        <button class="secondary" onclick="testMCP()">测试默认 MCP</button>
      </div>
    </div>

    <div class="module-view" data-module-view="automation">
      <div class="settings-block-head"><label>自动化任务（当前工作空间）</label><button class="secondary small" onclick="editScheduledTask()">新增任务</button></div>
      <input id="taskSearch" class="session-search" placeholder="搜索任务" oninput="renderScheduledTasks()" />
      <div id="scheduledTasks" class="tasks-list"><div class="hint">正在加载自动化任务...</div></div>
    </div>

    <div class="module-view" data-module-view="data">
      <div class="settings-block-head"><label>数据状态</label><button class="secondary small" onclick="loadDataStatus()">刷新数据状态</button></div>
      <div id="dataStatus" class="stat-grid"><div class="hint">正在加载数据状态...</div></div>
      <p class="hint">SQLite 单文件仍是主存储，旧 JSON 迁移入口保持兼容。备份恢复接口后续可继续接入。</p>
    </div>

    <div class="module-view" data-module-view="security">
      <div id="systemStatus" class="product-list"><div class="hint">正在加载系统状态...</div></div>
      <div class="settings-actions"><button class="secondary" onclick="setAuthToken()">登录 / 切换账号</button></div>
      <p class="hint">登录信息只保存在当前浏览器本地；API Key 和 MCP Token 前端脱敏显示。</p>
    </div>
  </section>
</div>
`;
