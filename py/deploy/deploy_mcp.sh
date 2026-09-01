#!/bin/bash
# Copyright (c) [2025] [liuyngchng@hotmail.com] - All rights reserved.
app='http_mcp'
docker stop ${app}
docker rm ${app}
#docker run -dit --name ${app} --network host --rm \
docker run -dit --name ${app}  \
  --security-opt seccomp=unconfined \
  -v /data/my_mcp:/opt/app \
  -p 19005:19000 \
  -p 19006:19001 \
  -e MODULE_NAME=${app} \
  llm_mcp:1.0

docker ps -a  | grep ${app} --color=always
docker logs -f ${app}
