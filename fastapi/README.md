测试过程
启动server
```shell
uvicorn compare:app --workers 1
```

测试结果如下所示
```shell
wrk -t20 -c200 -d60s --timeout 5s http://localhost:8000/async-test
Running 1m test @ http://localhost:8000/async-test
  20 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     3.01s    31.97ms   3.18s    90.68%
    Req/Sec    10.25     20.24    90.00     89.98%
  3800 requests in 1.00m, 690.23KB read
Requests/sec:     63.25
Transfer/sec:     11.49KB
(fast_api_env) rd@rd-t:~/workspace/my_fast_api$ wrk -t20 -c200 -d60s --timeout 5s http://localhost:8000/sync-test
Running 1m test @ http://localhost:8000/sync-test
  20 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     3.11s    24.72ms   3.14s    80.00%
    Req/Sec    15.58     19.09    90.00     86.42%
  760 requests in 1.00m, 138.05KB read
  Socket errors: connect 0, read 0, write 0, timeout 720
Requests/sec:     12.65
Transfer/sec:      2.30KB
```