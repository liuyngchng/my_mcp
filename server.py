#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
pip install mcp
pip install mcp[cli]

FastMCP quickstart example.
"""

import os
from mcp.server.fastmcp import FastMCP
from mcp.types import Request
from starlette.responses import JSONResponse

my_mcp = FastMCP(port=8001, stateless_http=True, json_response=True)  # 初始化 MCP 服务实例

@my_mcp.custom_route("/health", methods=["GET"])
async def health_check(request: Request):
    """健康检查端点"""
    print(f"trigger health_check, {request}")
    return JSONResponse({"status": "ok"})

@my_mcp.tool()
def get_desktop_files():
    """获取桌面上的文件列表"""
    print("trigger get_desktop_files")
    return os.listdir(os.path.expanduser("~/Desktop"))

if __name__ == "__main__":
    print("start mcp server (backend only)")
    # 通信协议：transport = 'stdio', 表示使用标准输入输出，也可替换为 HTTP 或 WebSocket
    my_mcp.run(transport='streamable-http')  # 添加 frontend=False
