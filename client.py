#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Run from the repository root:
    uv run examples/snippets/clients/streamable_basic.py
"""

import asyncio
import json
import logging.config
import time
from typing import Any, Generator

import requests
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from sys_init import init_yml_cfg

# 配置日志
logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

MCP_SERVER_ADDR = "http://localhost:8001/mcp"


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


def call_llm_with_retry(api: str, headers: dict, data: dict, cfg:dict, max_retries: int = 3) -> dict:
    """
    带重试机制的LLM调用
    """
    for attempt in range(max_retries):
        try:
            logger.info(f"第 {attempt + 1} 次尝试调用LLM API, proxies: {cfg['api']['proxy']}")
            response = requests.post(api, headers=headers, json=data, verify=False, proxies=cfg['api']['proxy'], timeout=30)
            logger.info(f"llm_response_status {response.status_code}")

            if response.status_code == 200:
                return response.json()
            else:
                logger.warning(f"LLM API 返回非200状态码: {response.status_code}")
                if attempt < max_retries - 1:
                    time.sleep(2 ** attempt)  # 指数退避

        except requests.exceptions.Timeout:
            logger.warning(f"LLM API 调用超时，尝试 {attempt + 1}/{max_retries}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)  # 指数退避
        except Exception as e:
            logger.warning(f"LLM API 调用失败: {str(e)}，尝试 {attempt + 1}/{max_retries}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)  # 指数退避

    # 所有重试都失败
    raise RuntimeError(f"LLM API 调用失败，已重试 {max_retries} 次")


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
    llm_tools = build_llm_tools(tools)
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
            logger.info(
                f"curl -ks --noproxy '*' -X POST {header_str} -d '{json.dumps(data, ensure_ascii=False)}' '{api}'")
            response_data = call_llm_with_retry(api, headers, data, cfg)
            logger.info(f"llm_response_data: {json.dumps(response_data, indent=2, ensure_ascii=False)}")
            if "error" in response_data:
                logger.error(f"LLM API 返回错误: {response_data['error']}")
                return f"LLM API 错误: {response_data['error']['message']}"

            # 检查 finish_reason
            finish_reason = response_data["choices"][0]["finish_reason"]

            if finish_reason == "stop":
                # 如果是 stop，返回最终回答
                final_response = response_data["choices"][0]["message"]["content"]
                logger.info(f"最终回答: {final_response}")
                return final_response
            elif finish_reason == "tool_calls":
                # 如果是 tool_calls，提取并执行所有工具调用
                tool_calls = extract_tool_calls(response_data)
                if tool_calls:
                    # 将工具调用消息添加到历史
                    tool_call_message = response_data["choices"][0]["message"]
                    messages.append(tool_call_message)

                    # 执行所有工具调用并收集结果
                    for tool_call in tool_calls:
                        # 调用MCP工具
                        tool_result = call_mcp_tool(
                            tool_name=tool_call["name"],
                            params=tool_call["arguments"]
                        )

                        # 添加工具执行结果到消息历史
                        tool_result_content = str(tool_result.content)
                        messages.append({
                            "role": "tool",
                            "content": tool_result_content,
                            "tool_call_id": tool_call["id"]
                        })

                    logger.info(f"所有工具调用结果已添加到消息历史，继续下一轮对话")
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


def auto_call_mcp_yield(question: str, cfg: dict) -> Generator[str, None, None]:
    """
    使用支持工具调用的LLM自动决策并调用MCP工具（流式版本）
    返回生成器，逐步产生结果
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
    llm_tools = build_llm_tools(tools)

    # 准备初始消息
    messages = [
        {"role": "system", "content": "你是一个智能助手，可以根据用户需求选择合适的工具。"},
        {"role": "user", "content": question}
    ]

    # 设置最大迭代次数，防止无限循环
    max_iterations = 10
    iteration = 0

    # 发送初始消息
    yield json.dumps({
        "type": "status",
        "content": f"开始处理您的问题: {question}",
        "iteration": iteration
    }, ensure_ascii=False)

    while iteration < max_iterations:
        iteration += 1
        logger.info(f"第 {iteration} 轮对话")

        # 发送状态更新
        yield json.dumps({
            "type": "status",
            "content": f"第 {iteration} 轮处理中...",
            "iteration": iteration
        }, ensure_ascii=False)

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
            logger.info(
                f"curl -ks --noproxy '*' -w'\\n' --tlsv1 -X POST {header_str} -d '{json.dumps(data, ensure_ascii=False)}' '{api}' | jq")

            response_data = call_llm_with_retry(api, headers, data, cfg)
            logger.info(f"llm_response_data: {json.dumps(response_data, indent=2, ensure_ascii=False)}")

            if "error" in response_data:
                logger.error(f"LLM API 返回错误: {response_data['error']}")
                yield json.dumps({
                    "type": "error",
                    "content": f"LLM API 错误: {response_data['error']['message']}"
                }, ensure_ascii=False)
                return

            # 检查 finish_reason
            finish_reason = response_data["choices"][0]["finish_reason"]

            if finish_reason == "stop":
                # 如果是 stop，返回最终回答
                final_response = response_data["choices"][0]["message"]["content"]
                logger.info(f"最终回答: {final_response}")

                # 发送最终结果
                yield json.dumps({
                    "type": "final",
                    "content": final_response,
                    "iteration": iteration
                }, ensure_ascii=False)
                return

            elif finish_reason == "tool_calls":
                # 如果是 tool_calls，提取并执行所有工具调用
                tool_calls = extract_tool_calls(response_data)
                if tool_calls:
                    # 将工具调用消息添加到历史
                    tool_call_message = response_data["choices"][0]["message"]
                    messages.append(tool_call_message)

                    # 发送工具调用信息
                    yield json.dumps({
                        "type": "tool_call",
                        "content": f"调用工具: {', '.join([tc['name'] for tc in tool_calls])}",
                        "tools": [tc["name"] for tc in tool_calls],
                        "iteration": iteration
                    }, ensure_ascii=False)

                    # 执行所有工具调用并收集结果
                    for tool_call in tool_calls:
                        # 发送工具执行开始信息
                        yield json.dumps({
                            "type": "tool_start",
                            "content": f"正在执行工具: {tool_call['name']}, {json.dumps(tool_call["arguments"], ensure_ascii=False)}",
                            "tool": tool_call["name"],
                            "iteration": iteration
                        }, ensure_ascii=False)

                        # 调用MCP工具
                        tool_result = call_mcp_tool(
                            tool_name=tool_call["name"],
                            params=tool_call["arguments"]
                        )

                        # 发送工具执行结果
                        tool_result_content = str(tool_result.content)
                        yield json.dumps({
                            "type": "tool_result",
                            "content": f"工具 {tool_call['name']} 执行完成",
                            "tool": tool_call["name"],
                            "result": tool_result_content[:200] + "..." if len(
                                tool_result_content) > 200 else tool_result_content,
                            "iteration": iteration
                        }, ensure_ascii=False)

                        # 添加工具执行结果到消息历史
                        messages.append({
                            "role": "tool",
                            "content": tool_result_content,
                            "tool_call_id": tool_call["id"]
                        })

                    logger.info(f"所有工具调用结果已添加到消息历史，继续下一轮对话")
                else:
                    # 如果无法提取工具调用，返回错误
                    logger.error("检测到工具调用但无法提取工具信息")
                    yield json.dumps({
                        "type": "error",
                        "content": "处理失败：无法提取工具调用信息"
                    }, ensure_ascii=False)
                    return
            else:
                # 其他情况，返回LLM的文本响应
                final_response = response_data["choices"][0]["message"]["content"]
                logger.info(f"最终回答 (finish_reason: {finish_reason}): {final_response}")

                yield json.dumps({
                    "type": "final",
                    "content": final_response,
                    "iteration": iteration
                }, ensure_ascii=False)
                return

        except Exception as e:
            logger.exception(f"call_llm_err")
            yield json.dumps({
                "type": "error",
                "content": f"LLM调用失败: {str(e)}"
            }, ensure_ascii=False)
            return

    # 如果达到最大迭代次数仍未得到最终回答
    yield json.dumps({
        "type": "error",
        "content": "处理超时，未能生成完整回答"
    }, ensure_ascii=False)


def build_llm_tools(tools):
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
    return llm_tools


def extract_tool_calls(content: dict) -> list[dict] | None:
    """
    提取多个工具调用信息
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

        tool_calls = []
        for tool_call in message["tool_calls"]:
            function = tool_call["function"]
            tool_calls.append({
                "id": tool_call["id"],
                "name": function["name"],
                "arguments": json.loads(function["arguments"])
            })
        return tool_calls

    except (KeyError, IndexError, json.JSONDecodeError) as e:
        logger.error(f"解析工具调用失败: {str(e)}")
        return None


if __name__ == "__main__":
    # 直接运行时的测试代码
    my_cfg = init_yml_cfg()
    my_question = "我想找个凉快点儿的城市去度假，酒店价格控制在300以下，帮我做个行程规划吧，另外根据天气看看我该带些什么衣服比较合适"

    # 测试流式输出
    logger.info("测试流式输出:")
    for chunk in auto_call_mcp_yield(my_question, my_cfg):
        my_data = json.loads(chunk)
        print(f"[{my_data['type']}] {my_data.get('content', '')}")

    # 测试普通输出
    logger.info("\n测试普通输出:")
    my_result = auto_call_mcp(my_question, my_cfg)
    logger.info(f"最终结果: {my_result}")