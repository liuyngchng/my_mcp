#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import json
import re

from sys_init import init_yml_cfg
from langchain_core.prompts import ChatPromptTemplate
from langchain_openai import ChatOpenAI
from utils import rmv_think_block, extract_md_content
import httpx
import logging.config
import time

logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

def get_model(cfg:dict):
    model = ChatOpenAI(
        api_key=cfg['api']['llm_api_key'],
        base_url=cfg['api']['llm_api_uri'],
        http_client=httpx.Client(verify=False, proxy=None),
        model=cfg['api']['llm_model_name']
    )
    return model

def classify_txt(labels: list, txt: str, cfg: dict, is_remote=True) -> str:
    """
    classify txt, multi-label can be obtained
    """
    max_retries = 6
    backoff_times = [5, 10, 20, 40, 80, 160]
    last_exception = None

    for attempt in range(max_retries + 1):
        try:
            if attempt > 0:
                wait_time = backoff_times[attempt - 1]
                logger.info(f"retry #{attempt} times after {wait_time}s")
                time.sleep(wait_time)

            label_str = ';\n'.join(map(str, labels))
            # logger.debug(f"classify_txt: {txt}")
            template = f'''对以下文本进行分类\n{label_str}\n文本：{txt}\n分类结果输出为单一分类标签文本，不要输出任何额外信息'''
            prompt = ChatPromptTemplate.from_template(template)
            # logger.info(f"prompt {prompt}")

            model = get_model(cfg)
            chain = prompt | model
            logger.info(f"submit_msg_to_llm, txt[{txt}], llm[{cfg['api']['llm_api_uri']}, {cfg['api']['llm_model_name']}]")
            response = chain.invoke({"txt": txt})
            output_txt = extract_md_content(rmv_think_block(response.content), "json")
            return output_txt

        except Exception as ex:
            last_exception = ex
            logger.error(f"retry_failed_in_classify_txt, retry_time={attempt}, {str(ex)}")
            if attempt < max_retries:
                continue
            logger.error(f"all_retries_exhausted_task_classify_txt_failed, {labels}, {txt}")
            raise last_exception

def replace_spaces(text):
    return re.sub(r'[ \t]+', ' ', text)


if __name__ == "__main__":
    logger.info("start")
    my_cfg = init_yml_cfg()
    logger.info(f"cfg: {my_cfg}")