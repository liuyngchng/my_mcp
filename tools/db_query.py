#!/usr/bin/env python3
# -*- coding: utf-8 -*-



def get_table_schema(schema_name: str, table_name: str) -> list:
    """获取目前 schema_name 中表名称为  table_name 的 schema， 输出为 json 格式"""
    table_schema = []
    return table_schema


def get_gen_sql_prompt(question: str, schema: str) -> str:
    """获取生成SQL的prompt"""
    # 此处应调用LLM生成SQL，以下是模拟返回
    prompt = f"基于表结构:\n{schema}\n生成查询SQL: {question}"
    # 实际调用LLM API并返回SQL
    return prompt


def exec_query(sql: str) -> dict:
    """执行sql查询， 输出为json格式"""
    result = {}
    return result


def render_chart(dt: dict, chart_type: str) -> dict:
    """
    将输入的数据，转换为chart js 格式的数据
    :param dt: 输入的数据， 格式为json格式
    :param chart_type: 图表类型， 支持的类型有：
        - line
        - bar
        - pie
        - radar
        - doughnut
        - polarArea
        - bubble
        - scatter
    :return: 输出为chart js 格式的数据
    """
    chart_js_dt = {}
    return chart_js_dt