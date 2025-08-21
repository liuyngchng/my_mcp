#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Run from the repository root:
    uv run examples/snippets/clients/streamable_basic.py
"""

import asyncio
import json
import logging.config
from typing import Any

import requests
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from sys_init import init_yml_cfg

logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

MCP_SERVER_ADDR="http://localhost:8001/mcp"

async def async_get_available_tools() -> list:
    """
    从 MCP Server 获取可用的工具列表
    """
    async with streamablehttp_client(MCP_SERVER_ADDR) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            tools_resp = await session.list_tools()  # 关键调用
            logger.info(f"mcp_available_tools: {tools_resp}")
            return [{
                "name": tool.name,
                "title": tool.title,
                "description": tool.description,
                "inputSchema": tool.inputSchema,
                "outputSchema": tool.outputSchema,
                "annotations": tool.annotations,
                "meta": tool.meta
            } for tool in tools_resp.tools]

async def async_call_mcp_tool(tool_name: str, params: dict = None) -> Any:
    """
    异步调用 MCP工具（使用流式 HTTP 客户端，MCP 客户端的类型需要与 MCP server 的类型相对应）
    :param tool_name: 工具名称
    :param params: 工具参数（字典格式）
    :return: 工具执行返回结果
    """
    logger.info(f"call_mcp_tool: {tool_name}, params: {params}")
    try:
        async with streamablehttp_client(MCP_SERVER_ADDR) as (read, write, _):
            async with ClientSession(read, write) as session:
                await session.initialize()
                result = await session.call_tool(tool_name, params or {})
                logger.info(f"call_mcp_tool_success: {tool_name} -> {result}")
                return result

    except Exception as e:
        logger.exception(f"call_mcp_tool_exception")
        raise RuntimeError(f"call_mcp_tool_exception: {str(e)}") from e

def call_mcp_tool(tool_name: str, params: dict) -> Any:
    """
    同步调用MCP工具（封装异步调用），便于在同步代码中使用
    """
    return asyncio.run(async_call_mcp_tool(tool_name, params))


def auto_call_mcp(question: str, cfg: dict) -> str:
    """
    使用支持工具调用的LLM自动决策并调用MCP工具
    """
    # 获取可用的MCP工具
    tools = asyncio.run(async_get_available_tools())
    # 读取LLM配置
    api = f"{cfg['api']['llm_api_uri']}/chat/completions"
    token = cfg["api"]["llm_api_key"]
    model_name = cfg["api"]["llm_model_name"]
    if not all([api, token, model_name]):
        raise ValueError("读取LLM配置出现错误")

    # 将MCP工具转换为LLM工具格式
    llm_tools = []
    for tool in tools:
        parameters = tool.get("inputSchema", {}).copy()
        if "title" in parameters:
            del parameters["title"]

        llm_tool = {
            "type": "function",
            "function": {
                "name": tool["name"],
                "description": tool.get("description", ""),
                "parameters": parameters
            }
        }
        llm_tools.append(llm_tool)

    # 准备初始消息
    messages = [
        {"role": "system", "content": "你是一个智能助手，可以根据用户需求选择合适的工具。"},
        {"role": "user", "content": question}
    ]
    # 设置最大迭代次数，防止无限循环
    max_iterations = 10
    iteration = 0
    while iteration < max_iterations:
        iteration += 1
        logger.info(f"第 {iteration} 轮对话")
        # 调用LLM API
        headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}"
        }
        data = {
            "model": model_name,
            "messages": messages,
            "tools": llm_tools,
            "stream": False
        }

        try:
            header_str = ""
            for k, v in headers.items():
                header_str += f' -H "{k}: {v}" '
            logger.info(f"curl -ks --noproxy '*' -X POST {header_str} -d '{json.dumps(data, ensure_ascii=False)}' {api}")
            response = requests.post(api, headers=headers, json=data, verify=False, proxies=None, timeout=10)
            logger.info(f"llm_response_status {response}")
            response_data = response.json()
            logger.info(f"llm_response_data: {json.dumps(response_data, indent=2, ensure_ascii=False)}")
            # 检查 finish_reason
            finish_reason = response_data["choices"][0]["finish_reason"]

            if finish_reason == "stop":
                # 如果是 stop，返回最终回答
                final_response = response_data["choices"][0]["message"]["content"]
                logger.info(f"最终回答: {final_response}")
                return final_response
            elif finish_reason == "tool_calls":
                # 如果是 tool_calls，提取并执行工具调用
                tool_call = extract_tool_call(response_data)
                if tool_call:
                    # 调用MCP工具
                    tool_result = call_mcp_tool(
                        tool_name=tool_call["name"],
                        params=tool_call["arguments"]
                    )

                    # 将工具调用和结果添加到消息历史中
                    tool_call_message = response_data["choices"][0]["message"]
                    call_msg = {"content": tool_call_message["content"], "role": tool_call_message["role"]}
                    messages.append(call_msg)

                    # 添加工具执行结果到消息历史
                    tool_result_content = str(tool_result.content)
                    messages.append({
                        "role": "tool",
                        "content": tool_result_content,
                        "tool_call_id": tool_call_message["tool_calls"][0]["id"]
                    })

                    logger.info(f"工具调用结果已添加到消息历史，继续下一轮对话")
                else:
                    # 如果无法提取工具调用，返回错误
                    logger.error("检测到工具调用但无法提取工具信息")
                    return "处理失败：无法提取工具调用信息"
            else:
                # 其他情况，返回LLM的文本响应
                final_response = response_data["choices"][0]["message"]["content"]
                logger.info(f"最终回答 (finish_reason: {finish_reason}): {final_response}")
                return final_response

        except Exception as e:
            logger.exception(f"call_llm_err")
            raise RuntimeError(f"LLM调用失败: {str(e)}") from e

    # 如果达到最大迭代次数仍未得到最终回答
    return "处理超时，未能生成完整回答"

def extract_tool_call(content: dict) -> dict | None:
    """
    content 内容格式如下所示：
    {
      "choices": [
        {
          "finish_reason": "tool_calls",
          "index": 0,
          "logprobs": null,
          "message": {
            "content": null,
            "function_call": null,
            "role": "assistant",
            "tool_calls": [
              {
                "function": {
                  "arguments": "{\"location\": \"\\u4f26\\u6566\", \"unit\": \"celsius\"}",
                  "name": "get_weather"
                },
                "id": "chatcmpl-tool-427dc9f32099443da4bf3c429b30b17d",
                "type": "function"
              }
            ]
          },
          "stop_reason": null
        }
      ],
      "model": "qwen2dot5-7b-chat",
      "object": "chat.completion"
    }
    Returns:
            字典格式 {"tool": "工具名", "arguments": {"参数": "值"}}
    """
    try:
        if "choices" not in content:
            return None

        choice = content["choices"][0]
        if choice.get("finish_reason") != "tool_calls":
            return None

        message = choice["message"]
        if "tool_calls" not in message or not message["tool_calls"]:
            return None

        tool_call = message["tool_calls"][0]
        function = tool_call["function"]

        return {
            "name": function["name"],
            "arguments": json.loads(function["arguments"])
        }
    except (KeyError, IndexError, json.JSONDecodeError) as e:
        logger.error(f"解析工具调用失败: {str(e)}")
        return None


async def test_client():
    """
    一个简单的客户端示例
    """
    # Connect to a streamable HTTP server
    logger.info(f"MCP_SERVER_ADDR {MCP_SERVER_ADDR}")
    async with streamablehttp_client(MCP_SERVER_ADDR) as (
        read_stream,
        write_stream,
        _,
    ):
        # Create a session using the client streams
        async with ClientSession(read_stream, write_stream) as session:
            # Initialize the connection
            await session.initialize()
            logger.info("list_available_tools")
            tools = await session.list_tools()
            logger.info(f"available_tools: {[tool.name for tool in tools.tools]}")
            tool_name = "get_desktop_files"
            logger.info(f"start_call_tool {tool_name}")
            result = await session.call_tool(tool_name)
            logger.info(f"call_result: {result}")

if __name__ == "__main__":
    # asyncio.run(test_client())
    my_cfg = init_yml_cfg()
    my_question = "我想找个凉快点儿的城市去度假，酒店价格控制在300以下，帮我做个行程规划吧，另外根据天气看看我该带些什么衣服比较合适"
    result = auto_call_mcp(my_question, my_cfg)
    logger.info(f"result_for_my_question: {result}")
