#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import json

from pydantic import BaseModel

import utils
from sys_init import init_yml_cfg
from utils import post_with_retry

import logging.config

logger = logging.getLogger(__name__)
db_cfg = init_yml_cfg()['api']

class DbInfo(BaseModel):
    name: str
    description: str

class TableInfo(BaseModel):
    name: str
    description: str

class TableColumnInfo(BaseModel):
    column_name: str
    data_type: str
    is_nullable: bool
    is_primary_key: bool
    comment: str

class TableSchemaInfo(BaseModel):
    table_name: str
    description: str
    columns: list[TableColumnInfo]

class ChartJsData(BaseModel):
    config: dict
    options: dict


def list_available_db_source() -> list[DbInfo]:
    """获取目前可用的数据源（数据库）列表"""
    db_source = utils.get_with_retry(
        uri=f"{db_cfg['tool_api_uri']}/data_source/list",
        headers={}, params={}, proxies=db_cfg['proxy']
    )
    logger.info(f"db_source {db_source}")
    db_list = []
    return db_list

def list_available_tables(db_source:str) -> list[TableInfo]:
    """获取指定数据源中的所有表清单信息"""
    tables = utils.get_with_retry(
        uri=f"{db_cfg['tool_api_uri']}/{db_source}/table/list",
        headers={}, params={}, proxies=db_cfg['proxy']
    )
    logger.info(f"tables {tables}")
    table_list = []
    return table_list

def get_table_schema(db_source: str, table_name: str) -> list:
    """获取目前 db_source 中表名称为  table_name 的 schema， 输出为 json 格式"""
    table_schema = utils.get_with_retry(
        uri=f"{db_cfg['tool_api_uri']}/{db_source}/{table_name}/schema",
        headers={}, params={}, proxies=db_cfg['proxy']
    )
    logger.info(f"table_schema {table_schema}")
    table_schema = []
    return table_schema

def execute_sql_query(sql: str) -> dict:
    """执行sql查询， 输出为json格式"""
    sql = sql.upper()
    if "INSERT" in sql or "UPDATE" in sql or "DELETE" in sql:
        logger.error(f"不支持的SQL语句: {sql}")
        return {}
    result = {}
    return result


def render_chart(chart_data: dict, chart_type: str, title: str, x_axis: str, y_axis: str) -> dict:
    """
    将输入的数据，转换为chart js 格式的数据
    :param chart_data: 输入的数据， 格式为json格式
    :param chart_type: 图表类型， 支持的类型有：
        - line
        - bar
        - pie
        - radar
        - doughnut
        - polarArea
        - bubble
        - scatter
    :param title: 图表标题
    :param x_axis: 图表x轴标题
    :param y_axis: 图表y轴标题
    :return: 输出为chart js 格式的数据
    """
    chart_js_dt = {}
    return chart_js_dt