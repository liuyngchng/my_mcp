#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import yaml
import os
import logging.config

logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)

__init_cfg__ = {}

def init_yml_cfg(cfg_file="cfg.yml")-> dict[str, any]:
    """
    yaml cfg.yml, you can copy cfg.yml.template and rewrite to your own cfg.yml
    """
    global __init_cfg__
    if __init_cfg__:
        logger.info(f"cfg_already_inited_return_variable__init_cfg__, {__init_cfg__}")
        return __init_cfg__
    # 检查配置文件
    if not os.path.exists(cfg_file):
        info = f"配置文件 {cfg_file} 不存在, 请根据根目录下的 {cfg_file}.template 设置环境配置信息，完成后将文件重命名为 {cfg_file}"
        print(info)
        exit(-2)
    # 读取配置
    with open(cfg_file, 'r', encoding='utf-8') as f:
        __init_cfg__ = yaml.safe_load(f)
    logger.info(f"init_cfg_from_cfg_file, {__init_cfg__}")
    return __init_cfg__


if __name__ == "__main__":
    my_cfg = init_yml_cfg()
    logger.info(f"cfg {my_cfg}")

    my_cfg1 = init_yml_cfg()
    logger.info(f"cfg {my_cfg1}")