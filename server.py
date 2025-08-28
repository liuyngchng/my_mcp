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
from tools import calculate_bmi

app = FastMCP(port=19001, stateless_http=True, json_response=True, host='0.0.0.0')
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
    """获取适合度假的城市列表"""
    logger.info("trigger_get_vocation_city_list")
    city_list = [
        "北京",
        "上海",
        "广州",
        "深圳",
        "成都",
        "重庆",
        "西安",
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

@app.resource("file:///path/to/{my_file}")
def read_file(my_file: str) -> str:
    """
    根据文件路径读取文件内容
    uri = "file:///path/to/file.txt"
    """
    with open(my_file.replace("file://", ""), "r") as f:
        return f.read()

@app.prompt()
def vacation_plan_prompt(city: str) -> str:
    """生成度假计划提示模板"""
    return f"请为{city}设计一个3天的度假计划，包含景点、餐饮和住宿建议"

def add_your_tools():
    # 使用 add_tool 方法添加工具（而不是使用装饰器）
    app.add_tool(
        calculate_bmi,
        name="calculate_bmi",
        description="根据体重(kg)和身高(m)计算身体质量指数(BMI)并返回分类",
        structured_output=False
    )

if __name__ == "__main__":
    add_your_tools()
    logger.info("start mcp server (backend only)")
    # 通信协议：transport = 'stdio', 表示使用标准输入输出，也可替换为 HTTP 或 WebSocket
    # app.run(transport='streamable-http')  # 添加 frontend=False
    starlette_app = app.streamable_http_app()
    import uvicorn

    uvicorn.run(
        starlette_app,
        host="0.0.0.0",
        port=19001,
        ssl_keyfile="./cert/srv.key",
        ssl_certfile="./cert/srv.crt",
        log_level="info"
    )
