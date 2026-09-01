#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Copyright (c) [2025] [liuyngchng@hotmail.com] - All rights reserved.

from typing import List, Optional
from fastapi import FastAPI, HTTPException, Depends, status
from pydantic import BaseModel, Field
from datetime import datetime
import uuid
import asyncpg
import asyncio
"""
浏览器访问  http://localhost:8000/docs
"""

# 创建 FastAPI 应用实例
app = FastAPI(
    title="待办事项API",
    description="一个简单的待办事项列表API示例",
    version="1.0.0"
)


# Pydantic 模型定义
class TodoCreate(BaseModel):
    title: str = Field(..., min_length=1, max_length=100, description="待办事项标题")
    description: Optional[str] = Field(None, max_length=500, description="待办事项描述")


class TodoUpdate(BaseModel):
    title: Optional[str] = Field(None, min_length=1, max_length=100, description="待办事项标题")
    description: Optional[str] = Field(None, max_length=500, description="待办事项描述")
    completed: Optional[bool] = Field(None, description="是否已完成")


class TodoInDB(BaseModel):
    id: str
    title: str
    description: Optional[str]
    completed: bool
    created_at: datetime
    updated_at: datetime

    class Config:
        orm_mode = True


# 数据库模拟（实际应用中应使用真实数据库）
class Database:
    def __init__(self):
        self.todos = {}

    async def get_todos(self) -> List[TodoInDB]:
        # 模拟异步数据库查询
        await asyncio.sleep(0.01)  # 模拟I/O延迟
        return list(self.todos.values())

    async def get_todo(self, todo_id: str) -> Optional[TodoInDB]:
        await asyncio.sleep(0.01)
        return self.todos.get(todo_id)

    async def create_todo(self, todo: TodoCreate) -> TodoInDB:
        await asyncio.sleep(0.01)
        todo_id = str(uuid.uuid4())
        now = datetime.now()
        db_todo = TodoInDB(
            id=todo_id,
            title=todo.title,
            description=todo.description,
            completed=False,
            created_at=now,
            updated_at=now
        )
        self.todos[todo_id] = db_todo
        return db_todo

    async def update_todo(self, todo_id: str, todo: TodoUpdate) -> Optional[TodoInDB]:
        await asyncio.sleep(0.01)
        if todo_id not in self.todos:
            return None

        db_todo = self.todos[todo_id]
        update_data = todo.dict(exclude_unset=True)

        for field, value in update_data.items():
            setattr(db_todo, field, value)

        db_todo.updated_at = datetime.now()
        self.todos[todo_id] = db_todo
        return db_todo

    async def delete_todo(self, todo_id: str) -> bool:
        await asyncio.sleep(0.01)
        if todo_id in self.todos:
            del self.todos[todo_id]
            return True
        return False


# 创建数据库实例
db = Database()


# 依赖项
async def get_db():
    return db


# 路由处理函数
@app.get("/", summary="API根目录", tags=["根目录"])
async def root():
    return {"message": "欢迎使用待办事项API", "docs": "/docs"}


@app.get("/todos/", response_model=List[TodoInDB], summary="获取所有待办事项", tags=["待办事项"])
async def read_todos(skip: int = 0, limit: int = 100, database: Database = Depends(get_db)):
    todos = await database.get_todos()
    return todos[skip:skip + limit]


@app.get("/todos/{todo_id}", response_model=TodoInDB, summary="获取特定待办事项", tags=["待办事项"])
async def read_todo(todo_id: str, database: Database = Depends(get_db)):
    todo = await database.get_todo(todo_id)
    if todo is None:
        raise HTTPException(status_code=404, detail="待办事项未找到")
    return todo


@app.post("/todos/", response_model=TodoInDB, status_code=status.HTTP_201_CREATED, summary="创建新待办事项",
          tags=["待办事项"])
async def create_todo(todo: TodoCreate, database: Database = Depends(get_db)):
    return await database.create_todo(todo)


@app.put("/todos/{todo_id}", response_model=TodoInDB, summary="更新待办事项", tags=["待办事项"])
async def update_todo(todo_id: str, todo: TodoUpdate, database: Database = Depends(get_db)):
    updated_todo = await database.update_todo(todo_id, todo)
    if updated_todo is None:
        raise HTTPException(status_code=404, detail="待办事项未找到")
    return updated_todo


@app.delete("/todos/{todo_id}", status_code=status.HTTP_204_NO_CONTENT, summary="删除待办事项", tags=["待办事项"])
async def delete_todo(todo_id: str, database: Database = Depends(get_db)):
    success = await database.delete_todo(todo_id)
    if not success:
        raise HTTPException(status_code=404, detail="待办事项未找到")
    return None


# 启动应用
if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)