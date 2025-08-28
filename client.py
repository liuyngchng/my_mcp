#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Run from the repository root:
    uv run examples/snippets/clients/streamable_basic.py
"""

import asyncio
import ssl
from datetime import datetime, timedelta
import json
import logging.config
import time
from typing import Any, Generator

import httpx
import requests
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from sys_init import init_yml_cfg

# 配置日志
logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

# 给出多个可用的 MCP 服务器地址
MCP_SERVER_ADDR_LIST = [
    "https://localhost:19001/mcp",

]

# 全局缓存
TOOLS_CACHE = {
    "tools": [],
    "tool_server_map": {},
    "last_updated": None
}

# 缓存有效期（分钟）
CACHE_EXPIRY_MINUTES = 30

def create_custom_http_client(**kwargs):
    """创建自定义的 HTTP 客户端，禁用 SSL 验证"""
    return httpx.AsyncClient(verify=False, **kwargs)

def get_tool_unique_name(server_index: int, tool_name: str) -> str:
    """为工具生成唯一名称，避免不同服务器的同名工具冲突"""
    return f"server{server_index}_{tool_name}"


def get_tool_call_name(unique_name: str) -> str:
    """解析工具的唯一名称，返回进行调用的工具名称"""
    return unique_name.split('_', 1)[1]


async def async_get_available_tools(force_refresh: bool = False) -> list:
    """
    从所有MCP Server获取可用的工具列表，支持缓存
    """
    global TOOLS_CACHE

    # 检查缓存是否有效
    current_time = datetime.now()  # 使用正确的datetime.now()
    if (not force_refresh and TOOLS_CACHE["last_updated"] and
            (current_time - TOOLS_CACHE["last_updated"]) < timedelta(minutes=CACHE_EXPIRY_MINUTES)):
        logger.info(f"使用缓存的工具列表，最后更新于 {TOOLS_CACHE['last_updated']}")
        return TOOLS_CACHE["tools"]

    all_tools = []
    tool_server_map = {}

    for index, server_addr in enumerate(MCP_SERVER_ADDR_LIST):
        try:
            async with streamablehttp_client(url=server_addr, timeout=30,  httpx_client_factory=create_custom_http_client) as (read, write, _):
                async with ClientSession(read, write) as session:
                    await session.initialize()
                    tools_resp = await session.list_tools()
                    logger.info(f"从服务器 {index}[{server_addr}] 获取到 {len(tools_resp.tools)} 个工具")
                    for tool in tools_resp.tools:
                        unique_name = get_tool_unique_name(index, tool.name)
                        tool_server_map[unique_name] = server_addr
                        all_tools.append({
                            "name": unique_name,
                            "title": tool.title,
                            "description": tool.description,
                            "inputSchema": tool.inputSchema,
                            "outputSchema": tool.outputSchema,
                            "annotations": tool.annotations,
                            "meta": tool.meta,
                            "server": server_addr
                        })
        except Exception as e:
            logger.error(f"连接服务器 {server_addr} 失败: {str(e)}")
            continue

    # 更新缓存
    TOOLS_CACHE["tools"] = all_tools
    TOOLS_CACHE["tool_server_map"] = tool_server_map
    TOOLS_CACHE["last_updated"] = current_time

    logger.info(f"总共获取到 {len(all_tools)} 个工具，已缓存， {TOOLS_CACHE}")
    return all_tools


async def async_call_mcp_tool(server_addr: str, call_tool_name: str, params: dict = None) -> Any:
    """
    异步调用 MCP工具，根据工具唯一名称找到对应的服务器
    :param server_addr Server addr of the tool
    :param call_tool_name: 工具调用名称（包含服务器哈希）
    :param params: 工具参数（字典格式）
    :return: 工具执行返回结果
    """
    logger.info(f"call_mcp_tool: {call_tool_name}@{server_addr}, params: {params}")
    try:
        async with streamablehttp_client(url=server_addr, timeout=30,  httpx_client_factory=create_custom_http_client) as (read, write, _):
            async with ClientSession(read, write) as session:
                await session.initialize()
                result = await session.call_tool(call_tool_name, params or {})
                logger.info(f"call_mcp_tool_success: {call_tool_name}@{server_addr} -> {result}")
                return result
    except Exception as e:
        logger.exception(f"call_mcp_tool_exception_for_tool {call_tool_name}@{server_addr}")
        raise RuntimeError(f"call_mcp_tool_exception: {str(e)}") from e


async def async_get_tool_server_addr(unique_tool_name):
    server_addr = TOOLS_CACHE["tool_server_map"].get(unique_tool_name)
    if not server_addr:
        # 如果缓存中没有，尝试刷新缓存
        await async_get_available_tools(force_refresh=True)
        server_addr = TOOLS_CACHE["tool_server_map"].get(unique_tool_name)
        if not server_addr:
            raise ValueError(f"未找到工具 {unique_tool_name} 对应的服务器")
    return server_addr


def call_mcp_tool(server_addr:str, call_tool_name: str, params: dict) -> Any:
    """
    将异步调用转换为同步调用， 同步调用MCP工具，便于在同步代码中使用
    """
    return asyncio.run(async_call_mcp_tool(server_addr, call_tool_name, params))


def call_llm_with_retry(api: str, headers: dict, data: dict, cfg:dict, max_retries: int = 3) -> dict:
    """
    带重试机制的LLM调用
    """
    proxies = cfg['api'].get('proxy', None)
    for attempt in range(max_retries):
        try:
            logger.info(f"第 {attempt + 1} 次尝试调用LLM API, proxies: {proxies}, data: {data}")
            response = requests.post(api, headers=headers, json=data, verify=False, proxies=proxies, timeout=30)
            logger.info(f"llm_response_status {response.status_code}")

            if response.status_code == 200:
                return response.json()
            else:
                logger.warning(f"LLM API 返回非200状态码: {response.status_code}, {response.json()}")
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
                    logger.info(f"function_call_tag= {tool_call_message['function_call']}")
                    if not tool_call_message["function_call"] or tool_call_message["function_call"] == "null":
                        tool_call_message["function_call"] = []
                    messages.append(tool_call_message)

                    # 执行所有工具调用并收集结果
                    for tool_call in tool_calls:
                        server_addr = asyncio.run(async_get_tool_server_addr(tool_call['name']))
                        tool_call_name = get_tool_call_name(tool_call['name'])
                        tool_result = call_mcp_tool(server_addr, tool_call_name, tool_call["arguments"])
                        tool_result_content = str(tool_result.content)
                        messages.append({
                            "role": "tool",
                            "content": tool_result_content,
                            "tool_call_id": tool_call["id"]
                        })

                    logger.info(f"所有工具调用结果已添加到消息历史，继续下一轮对话")
                else:
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
    logger.info(f"question: {question}, cfg {cfg}")
    mcp_tools = asyncio.run(async_get_available_tools())
    api = f"{cfg['api']['llm_api_uri']}/chat/completions"
    token = cfg["api"]["llm_api_key"]
    model_name = cfg["api"]["llm_model_name"]
    if not all([api, token, model_name]):
        raise ValueError("读取LLM配置出现错误")
    llm_tools = build_llm_tools(mcp_tools)
    messages = [
        {"role": "system", "content": "你是一个智能助手，可以根据用户需求选择合适的工具。"},
        {"role": "user", "content": question}
    ]
    max_iterations = 10
    iteration = 0

    yield json.dumps({
        "type": "status",
        "content": f"开始处理您的问题: {question}",
        "iteration": iteration
    }, ensure_ascii=False)

    while iteration < max_iterations:
        iteration += 1
        logger.info(f"第 {iteration} 轮对话")
        yield json.dumps({
            "type": "status",
            "content": f"第 {iteration} 轮处理中...",
            "iteration": iteration
        }, ensure_ascii=False)

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
                step_response = response_data["choices"][0]["message"]["content"]
                yield json.dumps({
                    "type": "status",
                    "content": step_response,
                    "iteration": iteration
                }, ensure_ascii=False)
                # 如果是 tool_calls，提取并执行所有工具调用
                tool_calls = extract_tool_calls(response_data)
                if tool_calls:
                    # 将工具调用消息添加到历史
                    tool_call_message = response_data["choices"][0]["message"]
                    if ("function_call" in tool_call_message and
                            (not tool_call_message["function_call"] or tool_call_message["function_call"] == "null")):
                        tool_call_message["function_call"] = []
                        logger.info(f"function_call_tag={tool_call_message['function_call']}")
                    if "content" in tool_call_message and (not tool_call_message["content"] or tool_call_message["content"] == "null"):
                        tool_call_message["content"] = ""
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
                        # 查找工具对应的服务器
                        server_addr = asyncio.run(async_get_tool_server_addr(tool_call['name']))
                        # 获取工具调用名称
                        tool_call_name = get_tool_call_name(tool_call['name'])
                        # 发送工具执行开始信息
                        yield json.dumps({
                            "type": "tool_start",
                            "content": f"正在执行工具: {tool_call_name}@{server_addr}, {json.dumps(tool_call["arguments"], ensure_ascii=False)}",
                            "tool": f"{tool_call_name}@{server_addr}",
                            "iteration": iteration
                        }, ensure_ascii=False)

                        # 调用MCP工具
                        tool_result = call_mcp_tool(server_addr, tool_call_name, tool_call["arguments"])

                        # 发送工具执行结果
                        tool_result_content = str(tool_result.content)
                        yield json.dumps({
                            "type": "tool_result",
                            "content": f"工具 {tool_call_name}@{server_addr} 执行完成",
                            "tool": f"{tool_call_name}@{server_addr}",
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
                "description": tool.get('description', ''),
                "parameters": parameters
            }
        }
        llm_tools.append(llm_tool)
    return llm_tools


def extract_tool_calls(content: dict) -> list[dict] | None:
    """
    提取多个工具调用信息
    现在LLM返回的tool应该都是全局唯一名称
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
            tool_name = function["name"]
            if tool_name not in TOOLS_CACHE["tool_server_map"]:
                asyncio.run(async_get_available_tools(force_refresh=True))
                if tool_name not in TOOLS_CACHE["tool_server_map"]:
                    logger.error(f"无法找到工具 {tool_name}")
                    continue

            tool_calls.append({
                "id": tool_call["id"],
                "name": tool_name,
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