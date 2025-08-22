#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
pip install mcp
pip install mcp[cli]

FastMCP quickstart example.
"""

import os
import logging.config
from mcp.server.fastmcp import FastMCP
from mcp.types import Request
from starlette.responses import JSONResponse

app = FastMCP(port=8001, stateless_http=True, json_response=True)  # 初始化 MCP 服务实例
logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)


@app.custom_route("/health", methods=["GET"])
async def health_check(request: Request):
    """健康检查端点"""
    logger.info(f"trigger_health_check, {request}")
    return JSONResponse({"status": "ok"})

@app.tool()
def get_desktop_files() -> list:
    """获取桌面上的文件列表"""
    logger.info("trigger_get_desktop_files")
    return os.listdir(os.path.expanduser("~/Desktop"))

@app.tool()
def get_stock_market_info() -> dict:
    logger.info("trigger_get_stock_market_info")
    return {
        "stock_name": "Apple Inc.",
        "stock_code": "AAPL",
        "stock_price": "123.45",
        "stock_volume": "1000000",
        "stock_change": "1.23%",
    }

@app.tool()
def get_hotel_by_city_and_price(city: str, max_price: int) -> dict:
    logger.info("trigger_get_hotel_by_city_and_price")
    return {
        "hotel_name": f"Grand Hotel {city}",
        "hotel_address": f"123 Main St, Anytown, {city}",
        "hotel_price": max_price - 10,
        "hotel_rating": "5",
    }


@app.tool()
def get_airline_info_by_city(city: str) -> dict:
    """获取某个城市的航班信息"""
    logger.info(f"trigger_get_airline_info_by_city({city})")
    airline_info = {
        "airline_name": "Delta Airlines",
        "airline_code": "Delta",
        "airline_price": "1000",
        "airline_rating": "5",
    }
    return airline_info

@app.tool()
def get_vocation_city_list() -> list:
    """
    获取适合度假的城市列表
    """
    logger.info("trigger_get_vocation_city_list")
    city_list = [
        "北京",
        "上海",
        "广州",
        "深圳",
        "成都",
        "重庆",
        "西安",
        "杭州",
        "武汉",
        "南京",
        "厦门",
        "青岛",
        "大连",
        "天津",
        "济南",
        "苏州",
        ]
    return city_list

@app.tool()
def get_weather_info_by_city(city: str) -> dict:
    """获取桌面上的文件列表"""
    logger.info(f"trigger_get_weather_info_by_location({city})")
    weather_info = {
        "temperature": "20℃",
        "wind direction":"south east",
        "wind speed": "3m/s",
        "humidity": "60%",
        "atmospheric pressure": "1000hPa",
        "visibility": "1000m",
        "cloudiness": "50%",
        "precipitation": "0.5mm",
        "dew point": "10℃",
        "uv index": "3",
        "snow depth": "0.5mm",
        "sunrise": "06:00",
        "sunset": "18:00",
    }
    return weather_info

@app.resource("file:///path/to/file.txt")
def read_file(uri: str) -> str:
    """根据文件路径读取文件内容"""
    with open(uri.replace("file://", ""), "r") as f:
        return f.read()

@app.prompt()
def vacation_plan_prompt(city: str) -> str:
    """生成度假计划提示模板"""
    return f"请为{city}设计一个3天的度假计划，包含景点、餐饮和住宿建议"

if __name__ == "__main__":
    logger.info("start mcp server (backend only)")
    # 通信协议：transport = 'stdio', 表示使用标准输入输出，也可替换为 HTTP 或 WebSocket
    app.run(transport='streamable-http')  # 添加 frontend=False
