#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Run from the repository root:
    uv run examples/snippets/clients/streamable_basic.py
"""

import asyncio
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client

MCP_SERVER_ADDR="http://localhost:8001/mcp"

async def client_test():
    # Connect to a streamable HTTP server
    print(f"MCP_SERVER_ADDR {MCP_SERVER_ADDR}")
    async with streamablehttp_client(MCP_SERVER_ADDR) as (
        read_stream,
        write_stream,
        _,
    ):
        # Create a session using the client streams
        async with ClientSession(read_stream, write_stream) as session:
            # Initialize the connection
            await session.initialize()
            print("list_available_tools")
            tools = await session.list_tools()
            print(f"available_tools: {[tool.name for tool in tools.tools]}")
            tool_name = "get_desktop_files"
            print(f"start_call_tool {tool_name}")
            result = await session.call_tool(tool_name)
            print(f"call result: {result}")


if __name__ == "__main__":
    asyncio.run(client_test())
