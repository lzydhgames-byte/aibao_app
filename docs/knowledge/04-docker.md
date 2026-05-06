# Docker 与容器

## 4.1 Docker
把软件 + 它的所有运行环境打包成"盒子"，运行时不污染你的系统。  
**类比**：搬一套完整西藏帐篷到你家——用完撤走，地面没动过。

## 4.2 容器 vs 虚拟机
- **虚拟机**：模拟整个 OS，几 GB，启动慢，隔离极强
- **容器**：共用宿主机内核，只打包应用，几十 MB，启动快

**类比**：虚拟机=独立楼；容器=公寓里租一间房。

## 4.3 镜像 vs 容器
- **镜像**：只读模板（如 `postgres:16`）
- **容器**：基于镜像跑起来的实例（如 `aibao-postgres-dev`）

**类比**：镜像=Word 模板；容器=基于模板新建并填了内容的文档。一个镜像可建无数容器。

## 4.4 docker-compose
用一个 YAML 描述"我要启哪些容器、怎么互联"，一行命令全部启动：
```bash
docker compose up -d        # 启动
docker compose down         # 停止（保留数据）
docker compose down -v      # 停止 + 删数据卷（彻底清空）
```

## 4.5 端口映射 `127.0.0.1:5432:5432`（安全）
绑定 `127.0.0.1` 而不是默认的 `0.0.0.0` —— **只允许本机访问**，不暴露给局域网/公网。  
如果写成 `5432:5432`，同 WiFi 下别人能直连你的数据库。

## 4.6 数据卷（volumes）
容器是临时的（删了就没），卷是持久的——把数据存外面：
```yaml
volumes:
  - aibao_pg_data:/var/lib/postgresql/data
```
容器删了数据还在；`down -v` 才删卷。

## 4.7 healthcheck
让 Docker 定期"问候"容器（如 `pg_isready`），判断**服务是否真能用**——不只是进程活着。  
其他依赖此服务的容器可以等到 healthy 才启动。

## 4.8 `docker exec`
在已运行的容器里执行命令——借用容器内的客户端：
```bash
docker exec aibao-postgres-dev pg_isready    # 不用本机装 psql
docker exec aibao-redis-dev redis-cli ping
```
