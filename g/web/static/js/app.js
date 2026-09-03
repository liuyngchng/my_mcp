// 聊天元素
const chatContainer = document.getElementById('chat-container');
const queryForm = document.getElementById('query-form');
const queryInput = document.getElementById('query-input');
const sendButton = document.getElementById('send-button');
const stopButton = document.getElementById('stop-button');

let isFetching = false;
let abortController = null;
let currentBotMessage = null;

const MAX_MESSAGES = 50;
let messages = [];
let sessionId = null;

function getSessionStorageKey() {
    return 'mcp_chat_session_id';
}

function loadSessionId() {
    try {
        sessionId = localStorage.getItem(getSessionStorageKey());
    } catch (e) {
        sessionId = null;
    }
}

function saveSessionId() {
    try {
        if (sessionId) {
            localStorage.setItem(getSessionStorageKey(), sessionId);
        }
    } catch (e) {
        // ignore
    }
}

function getStorageKey() {
    return 'mcp_chat_messages';
}

function saveMessages() {
    if (messages.length > MAX_MESSAGES) {
        messages = messages.slice(-MAX_MESSAGES);
    }
    localStorage.setItem(getStorageKey(), JSON.stringify(messages));
}

function loadMessages() {
    try {
        const raw = localStorage.getItem(getStorageKey());
        if (raw) {
            messages = JSON.parse(raw);
            if (messages.length > MAX_MESSAGES) {
                messages = messages.slice(-MAX_MESSAGES);
            }
            // 清理未完成的消息
            messages = messages.filter(function(m) {
                if (m.type === 'bot' && m.text.indexOf('typing-indicator') !== -1) {
                    return false;
                }
                return true;
            });
            localStorage.setItem(getStorageKey(), JSON.stringify(messages));
        }
    } catch (e) {
        messages = [];
    }
}

function restoreMessages() {
    chatContainer.innerHTML = '';
    messages.forEach(m => addMessageToDOM(m.text, m.type));
}

// 页面加载
window.onload = function() {
    loadMessages();
    loadSessionId();
    if (messages.length > 0) {
        restoreMessages();
    }
    queryInput.focus();
};

// 表单提交
queryForm.addEventListener('submit', async function(e) {
    e.preventDefault();
    if (isFetching) return;

    const query = queryInput.value.trim();
    if (!query) return;

    addMessage(query, 'user');
    queryInput.value = '';
    queryInput.focus();

    try {
        await fetchQueryData(query);
    } catch (error) {
        console.error('请求出错:', error);
        if (currentBotMessage) {
            updateBotMessage('抱歉，生成回复时出错，请重试。');
        }
        resetUI();
    }
});

// 停止按钮
stopButton.addEventListener('click', function() {
    if (abortController) {
        abortController.abort();
    }

    if (currentBotMessage) {
        const messageBubble = currentBotMessage.querySelector('.bot-message-bubble');
        if (messageBubble) {
            const typingIndicator = messageBubble.querySelector('.typing-indicator');
            if (typingIndicator) {
                typingIndicator.remove();
                const stopNotice = document.createElement('div');
                stopNotice.className = 'stop-notice';
                stopNotice.textContent = '已停止生成';
                messageBubble.appendChild(stopNotice);
            }
        }
    }
    resetUI();
});

let currentStatusMessage = null;

// 流式请求
async function fetchQueryData(query) {
    isFetching = true;
    sendButton.style.display = 'none';
    stopButton.style.display = 'inline-block';

    abortController = new AbortController();

    currentBotMessage = addMessage('<div class="typing-indicator"><span class="dot"></span><span class="dot"></span><span class="dot"></span> <span class="status-text">请求处理</span></div>', 'bot');
    currentStatusMessage = currentBotMessage.querySelector('.status-text');

    try {
        const response = await fetch('/api/chat', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'text/event-stream'
            },
            body: JSON.stringify({
                msg: query,
                session_id: sessionId || ''
            }),
            signal: abortController.signal
        });

        if (!response.ok) {
            throw new Error('网络请求失败');
        }
        if (!response.body) {
            throw new Error('不支持流式响应');
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let accumulatedText = '';
        let buffer = '';
        let currentEventType = '';

        while (true) {
            const { value, done } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });

            // 按行切分，保留最后一行（可能不完整）在 buffer 中
            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (let i = 0; i < lines.length; i++) {
                const line = lines[i];

                if (line.startsWith('event: ')) {
                    currentEventType = line.substring(7).trim();
                } else if (line.startsWith('data: ')) {
                    const data = line.substring(6);
                    if (data === '[DONE]' || data === '') continue;

                    if (currentEventType === 'status') {
                        // 原地更新状态文本，不新增元素
                        if (currentStatusMessage) {
                            currentStatusMessage.textContent = data;
                            currentStatusMessage.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                        }
                        currentEventType = '';
                    } else if (currentEventType === 'session') {
                        // 保存后端分配的会话 ID
                        sessionId = data;
                        saveSessionId();
                        currentEventType = '';
                    } else {
                        // 正文内容到达，清除状态行
                        if (currentStatusMessage) {
                            currentStatusMessage.remove();
                            currentStatusMessage = null;
                        }
                        // sseContent 使用 JSON 编码，解析后拼接到正文
                        let decoded = data;
                        try {
                            decoded = JSON.parse(data);
                        } catch (e) {
                            // 兼容旧格式（非 JSON 编码的纯文本）
                        }
                        accumulatedText += decoded;
                        updateBotMessage(accumulatedText);
                        currentEventType = '';
                    }
                }
                if (line === '') {
                    currentEventType = '';
                }
            }
        }

        // 处理 buffer 中剩余的最后一行
        if (buffer.trim()) {
            const lastLine = buffer.trim();
            if (lastLine.startsWith('data: ')) {
                const data = lastLine.substring(6);
                if (data !== '[DONE]' && data !== '' && currentEventType !== 'status') {
                    let decoded = data;
                    try {
                        decoded = JSON.parse(data);
                    } catch (e) {}
                    accumulatedText += decoded;
                    updateBotMessage(accumulatedText);
                }
            }
        }

        addCopyButton(currentBotMessage, accumulatedText);

    } catch (error) {
        if (error.name === 'AbortError') {
            console.log('请求已中止');
        } else {
            console.error('请求出错:', error);
            if (currentBotMessage) {
                updateBotMessage('请求失败，请重试。');
            }
        }
    } finally {
        if (currentStatusMessage) {
            currentStatusMessage.remove();
            currentStatusMessage = null;
        }
        resetUI();
    }
}

// 更新机器人消息
function updateBotMessage(text) {
    if (!currentBotMessage) return;

    const messageBubble = currentBotMessage.querySelector('.bot-message-bubble');
    if (messageBubble) {
        const sanitizedContent = DOMPurify.sanitize(
            marked.parse(text),
            { ADD_TAGS: ['canvas'], ADD_ATTR: ['id'] }
        );
        messageBubble.innerHTML = sanitizedContent;
        messageBubble.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }

    if (messages.length > 0 && messages[messages.length - 1].type === 'bot') {
        messages[messages.length - 1].text = text;
        saveMessages();
    }
}

// 添加消息到 DOM
function addMessageToDOM(text, type) {
    const messageContainer = document.createElement('div');
    messageContainer.classList.add('message-container');

    if (type === 'user') {
        messageContainer.classList.add('user-message-container');
        const sanitized = DOMPurify.sanitize(text);
        messageContainer.innerHTML = '<div class="message-bubble user-message-bubble">' + sanitized + '</div>';
    } else {
        messageContainer.classList.add('bot-message-container');
        const sanitized = DOMPurify.sanitize(
            marked.parse(text),
            { ADD_TAGS: ['canvas'], ADD_ATTR: ['id'] }
        );
        messageContainer.innerHTML =
            '<div class="bot-message-header"><span><i class="fas fa-robot"></i> AI助手</span></div>' +
            '<div class="message-bubble bot-message-bubble">' + sanitized + '</div>';
    }

    chatContainer.appendChild(messageContainer);
    messageContainer.scrollIntoView({ behavior: 'smooth' });
    return messageContainer;
}

// 添加消息
function addMessage(text, type) {
    messages.push({ text, type });
    saveMessages();
    return addMessageToDOM(text, type);
}

// 复制按钮
function addCopyButton(messageContainer, text) {
    const actionsContainer = document.createElement('div');
    actionsContainer.classList.add('message-actions');

    const copyButton = document.createElement('button');
    copyButton.classList.add('copy-button');
    copyButton.innerHTML = '<i class="fas fa-copy"></i> 复制';
    copyButton.onclick = function() {
        navigator.clipboard.writeText(text).then(() => {
            copyButton.innerHTML = '<i class="fas fa-check"></i> 已复制';
            setTimeout(() => {
                copyButton.innerHTML = '<i class="fas fa-copy"></i> 复制';
            }, 2000);
        });
    };

    actionsContainer.appendChild(copyButton);
    messageContainer.appendChild(actionsContainer);
}

// 新对话
async function newChat() {
    if (isFetching && abortController) {
        abortController.abort();
    }
    messages = [];
    saveMessages();
    // 清空后端会话历史
    if (sessionId) {
        try {
            await fetch('/api/chat/clear', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ session_id: sessionId })
            });
        } catch (e) {
            console.warn('清空后端会话历史失败:', e);
        }
    }
    sessionId = null;
    localStorage.removeItem(getSessionStorageKey());
    chatContainer.innerHTML = '';
    resetUI();
}

// 刷新工具
async function refreshTools() {
    try {
        const resp = await fetch('/api/refresh', { method: 'POST' });
        const data = await resp.json();
        alert('工具列表已刷新，共 ' + data.tools_count + ' 个工具');
    } catch (e) {
        alert('刷新失败: ' + e.message);
    }
}

// 重置 UI
function resetUI() {
    isFetching = false;
    sendButton.style.display = 'inline-block';
    stopButton.style.display = 'none';
    abortController = null;
}

// 新对话按钮
const newChatBtn = document.getElementById('newChat');
if (newChatBtn) {
    newChatBtn.addEventListener('click', newChat);
}

// 刷新工具按钮
const refreshBtn = document.getElementById('refreshTools');
if (refreshBtn) {
    refreshBtn.addEventListener('click', refreshTools);
}

// 键盘快捷键
queryInput.addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        queryForm.dispatchEvent(new Event('submit'));
    }
});