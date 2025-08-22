/bin/bash -c 'source ../mcp_py_env/bin/activate'
# for start application in docker container
MODULE="${MODULE_NAME:-http_mcp}"
echo "start module: ${MODULE}"
echo "current dir `pwd`"
echo "start server..."
/opt/mcp_py_env/bin/python server.py &
sleep 3
# 检查server.py是否仍在运行
if ! ps -p $! > /dev/null; then
    echo "Error: Server process failed to start!" >&2
    exit 1
fi
echo "Server started successfully with PID: $!"
echo "start http client"
/opt/mcp_py_env/bin/gunicorn --certfile ./cert/srv.crt --keyfile ./cert/srv.key --timeout 240 -w 1 --threads 8 -b 0.0.0.0:19000 ${MODULE}:app
