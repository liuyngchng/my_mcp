#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import json
import re
import sys
import logging.config
import time

import requests

logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

def extract_json(dt: str) -> str:
    # start from first '{', end with last '}'
    return re.sub(r'^.*?(\{.*\}).*$', r'\1', dt, flags=re.DOTALL)

def extract_md_content(raw_md: str, language: str) -> str:
    # extract  ```sql...``` code block
    pattern = rf"```{language}(.*?)```"
    match = re.search(pattern, raw_md, re.DOTALL)  # DOTALL模式匹配换行

    if match:
        txt = match.group(1)
        return txt.strip(" \n\t")
    else:
        raw_md = rmv_think_block(raw_md)
    return raw_md

def rmv_think_block(dt:str):
    dt = re.sub(r'<think>.*?</think>', '', dt, flags=re.DOTALL)
    return dt

def convert_list_to_md_table(my_list: list):

    headers = list(my_list[0].keys()) if my_list else []
    markdown_table = f"| {' | '.join(headers)} |\n| {' | '.join(['---'] * len(headers))} |\n"
    for item in my_list:
        row = " | ".join(str(item[h]).replace('\n', '<br>') for h in headers)
        markdown_table += f"| {row} |\n"
    return markdown_table

def convert_list_to_html_table(my_list: list):
    headers = list(my_list[0].keys()) if my_list else []
    html = ("<table>\n<thead>\n<tr>" + "".join(f"<th>{h}</th>" for h in headers)
            + "</tr>\n</thead>\n<tbody>")
    for item in my_list:
        row = "".join(f"<td>{str(item[h]).replace(chr(10), '<br>')}</td>" for h in headers)
        html += f"\n<tr>{row}</tr>"
    return html + "\n</tbody>\n</table>"

def get_console_arg1() -> int:
    # 检查命令行参数
    default_port = 19000
    max_port = 65535
    if len(sys.argv) <= 1:
        print(f"no_console_arg, using default {default_port}")
        return default_port
    try:
        console_port = int(sys.argv[1])  # 转换输入的端口参数
        if console_port < 1024 or console_port > 65535:
            print(f"port_out_of_range[1024, 65535]: {sys.argv[1]}, using max_port {max_port}")
            console_port = max_port
        return console_port
    except ValueError:
        print(f"invalid_port: {sys.argv[1]}, using default {default_port}")
    return default_port


def post_with_retry(uri: str, headers: dict, data: dict, proxies: str | None, max_retries: int = 3) -> dict:
    """
    带重试机制的LLM调用
    """
    for attempt in range(max_retries):
        try:
            logger.info(f"第 {attempt + 1} 次 post {uri}, proxies: {proxies}, data: {data}")
            response = requests.post(uri, headers=headers, json=data, verify=False, proxies=proxies, timeout=30)
            logger.info(f"llm_response_status {response.status_code}")

            if response.status_code == 200:
                logger.debug(f"post_response {json.dumps(response.json())}")
                return response.json()
            else:
                logger.warning(f"request API 返回非200状态码: {response.status_code}, {response.json()}")
                if attempt < max_retries - 1:
                    time.sleep(2 ** attempt)  # 指数退避

        except requests.exceptions.Timeout:
            logger.warning(f"request_API_timeout，retry {attempt + 1}/{max_retries}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)  # 指数退避
        except Exception as e:
            logger.warning(f"request_API_fail: {str(e)}，retry {attempt + 1}/{max_retries}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)  # 指数退避

    # 所有重试都失败
    raise RuntimeError(f"LLM API 调用失败，已重试 {max_retries} 次")

def get_with_retry(uri: str, headers: dict, params: dict, proxies: str | None, max_retries: int = 3) -> dict:
    """
    带重试机制的GET请求
    """
    for attempt in range(max_retries):
        try:
            logger.info(f"第 {attempt + 1} 次尝试调用GET API {uri}, proxies: {proxies}, params: {params}")
            response = requests.get(uri, headers=headers, params=params, verify=False, proxies=proxies, timeout=30)
            logger.info(f"get_response_status {response.status_code}")

            if response.status_code == 200:
                logger.debug(f"get_response {json.dumps(response.json(), ensure_ascii=False)}")
                return response.json()
            else:
                logger.warning(f"GET API 返回非200状态码: {response.status_code}")
                if attempt < max_retries - 1:
                    time.sleep(2 ** attempt)

        except requests.exceptions.Timeout:
            logger.warning(f"GET API 调用超时，尝试 {attempt + 1}/{max_retries}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)
        except Exception as e:
            logger.warning(f"GET API 调用失败: {str(e)}，尝试 {attempt + 1}/{max_retries}")
            if attempt < max_retries - 1:
                time.sleep(2 ** attempt)

    raise RuntimeError(f"GET API 调用失败，已重试 {max_retries} 次")

def build_curl_cmd(api, data, headers, proxies: dict | None):
    header_str = ""
    for k, v in headers.items():
        header_str += f' -H "{k}: {v}" '

    if proxies:
        curl_proxy = f"--proxy {proxies.get('http', proxies.get('https', None))}"
    else:
        curl_proxy = "--noproxy '*'"
    if 'https' in api:
        https_option = '-k --tlsv1'
    else:
        https_option = ''
    curl_log = f"curl -s {curl_proxy} -w'\\n' {https_option} -X POST {header_str} -d '{json.dumps(data, ensure_ascii=False)}' '{api}' | jq"
    return curl_log