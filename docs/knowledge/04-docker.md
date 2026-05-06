# Docker 与容器

---

## 4.1 Docker（容器引擎）

**一句话**：把一个软件 + 它需要的所有环境打包成一个独立的"盒子"，运行时不污染你的系统。

**对比"没 Docker 时"装 PostgreSQL**：
- 去官网下载、运行安装程序、配置环境变量、注册系统服务、改配置文件……
- 装完想完全卸载？几乎不可能干净

**有 Docker 时**：
- `docker run postgres:16` 一行命令，PG 就在你机器上跑起来
- 不要了 `docker rm` 一秒删干净
- **本机系统完全不留痕迹**

**生活类比**：你想体验"在西藏住一晚"——Docker 就是给你搬来一套完整的西藏帐篷（含床、锅具、海拔氧气），用完整套撤走，你家地面没动过。

**何时引入**：Task 0 安装本地 PG / Redis。

---

## 4.2 容器 vs 虚拟机

**一句话**：虚拟机模拟一整个操作系统（重，几 GB）；容器共用宿主机内核，只打包应用本身（轻，几十 MB）。

| | 虚拟机 | 容器 |
|---|---|---|
| 包含 | 整个操作系统 + 应用 | 仅应用 + 应用依赖 |
| 大小 | 几 GB ~ 几十 GB | 几 MB ~ 几百 MB |
| 启动 | 几十秒 ~ 分钟 | 毫秒 ~ 秒 |
| 隔离强度 | 极强（连内核都隔离） | 中等（共用内核，靠 namespace/cgroup） |
| 典型用途 | 跑完全不同的 OS、强安全隔离 | 应用部署、开发环境 |

**生活类比**：
- 虚拟机 = 在你家盖一栋独立楼，水电气都自带
- 容器 = 在公寓里租一间房，水电气共用大楼的，但门锁是自己的

**何时引入**：Task 0 决定用 Docker 而不是装本地 PG。

---

## 4.3 镜像 vs 容器

**一句话**：镜像是"模板"（只读），容器是"运行的实例"（可写）。

```
postgres:16          ← 这是镜像（一份只读模板，从 Docker Hub 拉下来）
    ↓ docker run
aibao-postgres-dev   ← 这是容器（一个跑着的进程，可读可写）
```

**类比**：
- 镜像 = Word 模板文件
- 容器 = 你基于模板新建并填了内容的 Word 文档

可以从同一个镜像创建无数个容器，互不干扰。

**何时引入**：Task 0 docker-compose 拉 `postgres:16-alpine` 镜像启动容器。

---

## 4.4 docker-compose

**一句话**：用一个 YAML 文件描述"我要启哪些容器、怎么互联"，一行命令全部启动。

**没有 compose 时**：
```bash
docker run -d --name pg ... postgres:16
docker run -d --name redis ... redis:7
docker network ...
```

**有 compose 时**：写一份 `docker-compose.yml`，然后：
```bash
docker compose up -d        # 启动全部
docker compose down         # 停止全部
docker compose down -v      # 停止 + 删数据卷（彻底清空）
```

**关键概念**：
- **services**：每个 service 对应一个容器
- **volumes**：数据卷——容器删了数据还在；`down -v` 才会删数据
- **ports**：端口映射，把容器内端口映射到宿主机
- **healthcheck**：容器健康检查
- **networks**：自动建网，同一 compose 文件里的 service 互相用 service 名通信

**何时引入**：Task 0 创建 `docker-compose.dev.yml`。

---

## 4.5 端口映射 `127.0.0.1:5432:5432`（安全细节）

**一句话**：把容器内 5432 端口映射到**本机 127.0.0.1 的 5432**，**不暴露给局域网/公网**。

```yaml
ports:
  - "127.0.0.1:5432:5432"
```

**对比常见的错误写法 `5432:5432`**（隐式 `0.0.0.0:5432:5432`）：
- 默认绑定到 `0.0.0.0`（所有网络接口）—— **同 WiFi 下别人能直连你的 PG！**

**强制 `127.0.0.1:` 前缀**：只允许本机访问。安全多得多。

**生活类比**：开发数据库就像家里冰箱里的"试做菜"——只想自己尝，不想被同小区的人随便走进来吃。`127.0.0.1` 就是把冰箱锁在卧室。

**何时引入**：Task 0 docker-compose.dev.yml。

---

## 4.6 数据卷（volumes）

**一句话**：容器是临时的（删了就没），数据卷是持久的——把数据存外面。

```yaml
volumes:
  - aibao_pg_data:/var/lib/postgresql/data
```

意思：把容器内 `/var/lib/postgresql/data`（PG 存数据的地方）**挂载**到一个名叫 `aibao_pg_data` 的卷上。

**容器删了数据还在**——下次创建新容器挂同一个卷，PG 启动时看到老数据，继续用。

**彻底清空**：`docker compose down -v` 才会同时删卷。

**何时引入**：Task 0 docker-compose 配置。

---

## 4.7 healthcheck

**一句话**：让 Docker 定期检查"容器是否真的能服务"，而不只是"进程是否还活着"。

```yaml
healthcheck:
  test: ["CMD-SHELL", "pg_isready -U aibao -d aibao"]
  interval: 5s
  timeout: 3s
  retries: 5
```

**为什么需要**：
- 进程在跑 ≠ 服务正常。比如 PG 进程启动了，但内部初始化还没完，连接会失败
- healthcheck 通过实际"问候"来判断（PG 用 `pg_isready` 测连接；Redis 用 `ping` 看是否回 `PONG`）
- 其他依赖此服务的容器可以等到 healthy 后才启动

**何时引入**：Task 0 docker-compose.dev.yml。

---

## 4.8 `docker exec` 借用容器内的客户端

**一句话**：在已运行的容器里执行一条命令。

```bash
docker exec aibao-postgres-dev pg_isready -U aibao -d aibao
docker exec aibao-redis-dev redis-cli ping
```

**好处**：不用在本机装 `psql` / `redis-cli`，借用容器自带的客户端。

**何时引入**：Task 0 验证容器健康。
