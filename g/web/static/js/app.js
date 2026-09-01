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

// 流式请求
async function fetchQueryData(query) {
    isFetching = true;
    sendButton.style.display = 'none';
    stopButton.style.display = 'inline-block';

    abortController = new AbortController();

    // 加载中动画
    currentBotMessage = addMessage('<div class="typing-indicator"><span></span><span></span><span></span> 思考中...</div>', 'bot');

    try {
        const response = await fetch('/api/chat', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'text/event-stream'
            },
            body: JSON.stringify({
                msg: query
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

        while (true) {
            const { value, done } = await reader.read();
            if (done) break;

            const chunk = decoder.decode(value, { stream: true });
            const lines = chunk.split('\n');

            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    const data = line.substring(6);
                    if (data === '[DONE]' || data === '') continue;

                    accumulatedText += data;
                    updateBotMessage(accumulatedText);
                }
            }
        }

        // 添加复制按钮
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