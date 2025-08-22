#!/bin/bash
LLM_PY_ENV="llm_py_env"
# 检查文件夹是否存在
if [ ! -d "${LLM_PY_ENV}" ]; then
    echo "错误：未找到 ${LLM_PY_ENV} 目录，退出执行"
    exit 1
fi
docker build --rm -f ./Dockerfile_mcp ./ -t llm_mcp:1.0
docker images | grep '<none>' | awk -F ' ' '{print $3}' | xargs docker rmi -f
docker images
