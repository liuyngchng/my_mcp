#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import re

from sys_init import init_yml_cfg
from langchain_openai import ChatOpenAI
import httpx
import logging.config

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


def replace_spaces(text):
    return re.sub(r'[ \t]+', ' ', text)


if __name__ == "__main__":
    logger.info("start")
    my_cfg = init_yml_cfg()
    logger.info(f"cfg: {my_cfg}")