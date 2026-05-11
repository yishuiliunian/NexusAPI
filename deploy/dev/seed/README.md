## dev 环境的 seed

不用 seed.sql。dev.sh 直接调 `backend/cmd/e2e-seed` 这个工具
（它能 bcrypt 哈希密码、用领域类型直接落库，与生产路径一致）。

调用形式：

```
go run ./backend/cmd/e2e-seed \
    --postgres-dsn "postgres://nexusapi:nexusapi_dev@127.0.0.1:${POSTGRES_PORT}/nexusapi?sslmode=disable" \
    --upstream-url "http://127.0.0.1:${UPSTREAM_PORT}" \
    --reset
```

种子内容：
- admin 账号：`admin@e2e.test` / `admin12345`
- 普通用户：`alice@e2e.test` / `user12345`
- 一条 claude provider channel + 一条 openai-compat channel（指向 upstream-mock）
- 模型价格表 + 一条兑换码 + 一份订阅套餐
