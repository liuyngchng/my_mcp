#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Copyright (c) [2025] [liuyngchng@hotmail.com] - All rights reserved.

from fastapi import FastAPI
import asyncio
import time
import httpx
import logging.config
"""
pip install fastapi httpx asyncio concurrent_log_handler
"""

app = FastAPI(title="异步 vs 同步性能演示")

logging.config.fileConfig('logging.conf', encoding="utf-8")
logger = logging.getLogger(__name__)


# 模拟一个耗时的外部I/O操作（如数据库查询、第三方API调用）
async def mock_io_operation_async(duration: float = 1.0):
    """异步模拟I/O操作"""
    await asyncio.sleep(duration)  # 非阻塞等待
    return {"status": "ok", "duration": duration}


def mock_io_operation_sync(duration: float = 1.0):
    """同步模拟I/O操作"""
    time.sleep(duration)  # 阻塞等待
    return {"status": "ok", "duration": duration}


# 异步端点
@app.get("/async-test")
async def async_test():
    """异步处理：在等待时事件循环可以处理其他请求"""
    logger.info("trigger-async-test")
    result = await mock_io_operation_async(3.0)
    logger.info("trigger-async-test-end")
    return {"message": "异步请求完成", **result}


# 同步端点
@app.get("/sync-test")
def sync_test():
    """同步处理：在等待时会阻塞整个线程"""
    logger.info("trigger-sync-test")
    result = mock_io_operation_sync(3.0)
    logger.info("trigger-sync-test-end")
    return {"message": "同步请求完成", **result}


# 更真实的示例：并发调用多个外部API
@app.get("/multiple-async-requests")
async def multiple_async_requests():
    """并发发起多个异步请求"""
    async with httpx.AsyncClient() as client:
        # 创建三个任务
        task1 = client.get("https://httpbin.org/delay/1", timeout=10)
        task2 = client.get("https://httpbin.org/delay/2", timeout=10)
        task3 = client.get("https://httpbin.org/delay/3", timeout=10)

        # 并发执行所有任务
        responses = await asyncio.gather(task1, task2, task3)

    total_duration = max([r.json().get('delay', 0) for r in responses])
    return {
        "message": f"并发请求完成，总耗时约 {total_duration} 秒",
        "results": [r.json() for r in responses]
    }


@app.get("/")
async def root():
    return {
        "message": "异步性能演示API",
        "endpoints": {
            "异步测试": "/async-test",
            "同步测试": "/sync-test",
            "并发异步请求": "/multiple-async-requests"
        }
    }

"""
uvicorn compare:app --workers 1
限制多线程，否则 uvicorn 会启动多线程，每个线程处理单独的请求，无法看到同步和异步的区别
测试脚本
# 测试异步接口，设置 5 秒超时
wrk -t20 -c200 -d60s --timeout 5s http://localhost:8000/async-test

# 测试同步接口，设置 5 秒超时
wrk -t20 -c200 -d60s --timeout 5s http://localhost:8000/sync-test
"""
if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)