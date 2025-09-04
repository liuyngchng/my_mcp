#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Copyright (c) [2025] [liuyngchng@hotmail.com] - All rights reserved.

import asyncio
import aiohttp
import requests
import time

BASE_URL = "http://localhost:8000"


async def test_async_concurrency():
    """测试异步端点的并发能力"""
    start_time = time.time()
    async with aiohttp.ClientSession() as session:
        tasks = []
        for _ in range(10):  # 同时发起10个请求
            task = asyncio.create_task(session.get(f"{BASE_URL}/async-test"))
            tasks.append(task)

        responses = await asyncio.gather(*tasks)

    end_time = time.time()
    total_time = end_time - start_time
    print(f"✅ 异步测试： 10个请求总耗时: {total_time:.2f} 秒")
    return total_time


def test_sync_concurrency():
    """测试同步端点的并发能力"""
    start_time = time.time()
    for _ in range(10):  # 顺序发起10个请求
        response = requests.get(f"{BASE_URL}/sync-test")
    end_time = time.time()
    total_time = end_time - start_time
    print(f"⏳ 同步测试： 10个请求总耗时: {total_time:.2f} 秒")
    return total_time


async def main():
    print("开始性能对比测试...")
    print("=" * 50)

    # 测试异步
    async_time = await test_async_concurrency()

    # 等待一下以免影响结果
    await asyncio.sleep(2)

    # 测试同步
    sync_time = test_sync_concurrency()

    print("=" * 50)
    print(f"🎯 性能提升: {sync_time / async_time:.1f} 倍 faster!")


if __name__ == "__main__":
    asyncio.run(main())