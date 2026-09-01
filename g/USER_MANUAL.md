# MCP Chat 使用手册

## 简介

MCP Chat 是一个基于 MCP（Model Context Protocol）的自然语言查询工具。通过连接 LLM 大模型和 MCP 服务器，你可以用自然语言（例如"查询 xxxx 公司的股东"）直接查询企业信息，无需手动调用 API。

**核心能力：**
- 内置多个 MCP 数据源（企业工商、风险、知识产权、经营、司法案例等）
- 支持 OpenAI 兼容接口的 LLM（DeepSeek、GPT 等）
- 纯静态编译，单文件运行，无需安装任何依赖

---

## 快速开始

### 1. 准备配置文件

将 `cfg.yml.template` 复制为 `cfg.yml`，放在程序同目录下：

```bash
cp cfg.yml.template cfg.yml
```

### 2. 编辑 cfg.yml

```yaml
server:
  port: 19002          # 服务端口
  debug: true          # 调试模式（true 打印更多日志）
  title: "AI 助手"     # 页面标题，默认 "AI 助手"

llm:
  api_uri: "https://your-llm-api.com/v1"   # LLM 接口地址（OpenAI 兼容）
  api_key: "sk-xxxxxxxx"                   # API Key
  model_name: "deepseek-v4-flash"          # 模型名称
  verify_ssl: false                        # 是否验证 SSL 证书
  enable_proxy: false                      # 是否使用 HTTP_PROXY 代理

mcp:
  verify_ssl: false
  enable_proxy: true
  auth_token: "your-mcp-token"             # MCP 服务认证 token（自动加 Bearer 前缀）
```

### 3. 运行

**Windows：** 双击 `mcp-chat_YYYYMMDD_windows_amd64.exe`，或命令行运行：

```cmd
mcp-chat_YYYYMMDD_windows_amd64.exe
```

**Linux：** 命令行运行：

```bash
./mcp-chat_YYYYMMDD_linux_amd64
```

启动后打开浏览器访问 `http://localhost:19002`。

---

## 使用方式

### 聊天查询

在输入框中用自然语言提问，例如：

- "查询阿里巴巴的股东信息"
- "腾讯有哪些对外投资"
- "查一下字节跳动的专利信息"
- "华为最近有什么招投标"

系统会自动选择合适的 MCP 数据源查询，你可以在界面上看到实时进度：

- **正在分析您的问题...** — LLM 正在理解你的意图
- **📡 正在查询 qcc-company → search_company ...** — 正在调用具体的 MCP 工具
- **✅ qcc-company 查询完成，正在分析结果...** — 数据已返回，LLM 正在整理回答

### 新对话

点击右上角 **新对话** 按钮清空当前对话历史。

### 刷新工具

如果 MCP 服务器更新了工具列表，点击右上角 **刷新工具** 重新加载。

---

## 系统状态

程序启动时会自动初始化所有 MCP 连接。如果某个 MCP 服务器连接失败，会在日志中记录，不影响其他服务器的正常使用。

可访问 `http://localhost:19002/health` 查看当前状态：

```json
{
  "status": "ok",
  "tools_count": 149,
  "mcp_servers": 7
}
```

---

## 日志

运行日志保存在程序同目录下的 `logs/` 文件夹中：

- `logs/app.2026-09-01-22.log` — 按小时滚动
- `logs/app.log` — 指向最新日志文件的快捷方式

日志同时输出到控制台和文件。

---

## 编译打包

### Windows（build.bat）

```cmd
build.bat                # 使用当天日期作为版本号
build.bat 1.0.0          # 指定版本号
```

产物输出到 `build/` 目录：
- `mcp-chat_{version}_windows_amd64.exe`
- `mcp-chat_{version}_windows_arm64.exe`

### Linux / macOS（build.sh）

```bash
./build.sh               # 编译 Linux elf（amd64 + arm64）
./build.sh 1.0.0         # 指定版本号
./build.sh --windows     # 同时交叉编译 Windows exe
./build.sh 1.0.0 --windows
```

产物输出到 `build/` 目录。

---

## 部署

1. 将编译好的可执行文件拷贝到目标机器
2. 在同目录下创建 `cfg.yml` 配置文件
3. 运行即可

程序是纯静态编译，不依赖任何系统库或运行时环境。

---

## 常见问题

**Q: 双击 exe 闪退？**

A: 查看 `logs/` 目录下的日志文件，确认 `cfg.yml` 是否存在且配置正确。

**Q: 提示 401 认证失败？**

A: MCP 的 `auth_token` 已过期或无效，需要重新获取 token 并更新 `cfg.yml`。

**Q: 如何修改 MCP 数据源？**

A: 编辑程序同目录下的 `mcp_servers.json`，重启程序即可。