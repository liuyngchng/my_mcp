#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Run from the repository root:
    uv run examples/snippets/clients/streamable_basic.py
"""

import asyncio
import json
from typing import Any

from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client

MCP_SERVER_ADDR="http://localhost:8001/mcp"

MCP_TOOLS = [
    {
        "name": "get_weather_info_by_location",
        "description": "查询指定地点的天气信息",
        "parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
    },
    {
        "name": "get_desktop_files",
        "description": "获取当前用户桌面文件列表",
        "parameters": {"type": "object", "properties": {}}  # 无参数
    }
]

async def client_test():
    # Connect to a streamable HTTP server
    print(f"MCP_SERVER_ADDR {MCP_SERVER_ADDR}")
    async with streamablehttp_client(MCP_SERVER_ADDR) as (
        read_stream,
        write_stream,
        _,
    ):
        # Create a session using the client streams
        async with ClientSession(read_stream, write_stream) as session:
            # Initialize the connection
            await session.initialize()
            print("list_available_tools")
            tools = await session.list_tools()
            print(f"available_tools: {[tool.name for tool in tools.tools]}")
            tool_name = "get_desktop_files"
            print(f"start_call_tool {tool_name}")
            result = await session.call_tool(tool_name)
            print(f"call result: {result}")

async def async_get_available_tools() -> list:
    """动态获取可用工具列表"""
    async with streamablehttp_client(MCP_SERVER_ADDR) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            tools_resp = await session.list_tools()  # 关键调用
            return [{
                "name": tool.name,
                "description": tool.description,
                "parameters": json.loads(tool.parameters_schema)  # 解析参数模式
            } for tool in tools_resp.tools]


async def async_call_mcp_tool(tool_name: str, params: dict = None) -> Any:
    """
    异步调用MCP工具（使用流式HTTP客户端）
    :param tool_name: 工具名称
    :param params: 工具参数（字典格式）
    :return: 工具执行结果
    """
    MCP_SERVER_ADDR = "http://localhost:8001/mcp"
    print(f"调用MCP工具: {tool_name} 参数: {params}")

    try:
        async with streamablehttp_client(MCP_SERVER_ADDR) as (read, write, _):
            async with ClientSession(read, write) as session:
                await session.initialize()

                # 获取可用工具列表（可选）
                # tools = await session.list_tools()
                # print(f"可用工具: {[t.name for t in tools.tools]}")

                # 执行工具调用
                result = await session.call_tool(tool_name, params or {})
                print(f"工具调用成功: {tool_name} -> {result}")
                return result

    except Exception as e:
        print(f"MCP调用失败: {str(e)}")
        raise RuntimeError(f"MCP服务调用失败: {str(e)}") from e

# 同步封装函数（便于在同步代码中使用）
def call_mcp_tool(tool_name: str, params: dict, cfg:dict) -> Any:
    """
    同步调用MCP工具（封装异步调用）
    """
    return asyncio.run(async_call_mcp_tool(tool_name, params))


# 新增函数
def auto_call_mcp(question: str, cfg: dict) -> str:
    """
    LLM自动选择并调用MCP工具
    :param question: 用户问题
    :param cfg: 系统配置
    :return: 工具执行结果
    """
    # 1. 构造工具选择指令
    tools = asyncio.run(async_get_available_tools())
    # 构造LLM提示词（包含工具描述）
    prompt = f"""
        请根据用户问题选择工具并生成JSON调用指令。可用工具：
        {json.dumps(tools, indent=2, ensure_ascii=False)}

        用户问题：{question}
        输出格式：{{"tool": "工具名", "arguments": {{...}}}}
        """

    # 2. LLM生成工具调用指令
    model = get_model(cfg, True)
    response = model.invoke(prompt)
    call_cmd = json.loads(extract_tool_name(response.content, "json"))

    # 3. 执行工具调用
    return call_mcp_tool(
        tool_name=call_cmd["tool"],
        params=call_cmd["arguments"],
        cfg=cfg
    )

def get_model(cfg:dict, is_remote=True):
    """
    获取LLM模型
    :param cfg: 系统配置
    :param is_remote: 是否远程模型
    :return: 模型对象
    """
    if is_remote:
        # 远程模型
        # from mcp.client.remote_model import RemoteModel
        # return RemoteModel(cfg["model_url"])
        print("return_remote_model")
    else:
        # 本地模型
        # from mcp.client.local_model import LocalModel
        # return LocalModel(cfg["model_path"])
        print("return_local_model")

def extract_tool_name(content: str, key: str) -> str:
    return None;

if __name__ == "__main__":
    asyncio.run(client_test())
