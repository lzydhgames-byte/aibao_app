# aibao-server

爱宝后端服务。

## 本地开发

前置：Go 1.22+（项目当前在 Go 1.24.2 上开发，见 `go.mod` 的 `go` 指令；任何 1.22 及以上版本均可编译）、Docker（用于 testcontainers 集成测试）、PostgreSQL 16、Redis 7、`migrate` CLI（`go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`）。

仓库根目录已提供 `docker-compose.dev.yml`，可一键启动 PG+Redis：

    docker compose -f ../docker-compose.dev.yml up -d

之后：

    make migrate-up
    make run-dev

健康检查：

    curl localhost:8080/health
    curl localhost:8080/ready
    curl localhost:8080/metrics

## 测试

    make test                # 单测
    make test-integration    # 集成测试（需要 Docker）
    make lint
