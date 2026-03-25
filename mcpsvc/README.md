# MCP 服务（独立进程）

```bash
go run ./telegram/cmd/mcp-telegram -listen :8765
```

## mcp-exec（仅 Docker Compose）

在 **宿主机** 上需要：Docker、本仓库的 sandbox 镜像、`docker.sock` 挂进 mcp-exec 容器。命令在 **长期 sandbox 容器**里执行（`docker exec`），不是在本机进程里跑 shell。

### 环境变量

在运行 `docker compose` 的 shell 里导出（**同一终端、在 compose 之前**）：

| 变量 | 含义 |
|------|------|
| `EXEC_HOST_REPO_DIR` | **宿主机**上本仓库根目录的绝对路径，与 compose volume **左侧**一致（Mac 上一般为 `/Users/...`）。 |
| `MCP_EXEC_REPO_MOUNT` | **mcp-exec 容器内**仓库挂载点，与 volume **右侧**一致（绝对路径）。 |
| `EXEC_WORKSPACE_DIRS` | **mcp-exec 容器内** workspace（可逗号分隔多个），须为 `{MCP_EXEC_REPO_MOUNT}/...` 下**已存在**的目录。 |
| `EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX` | **Docker Desktop（Mac）必填 `1`**：嵌套 `docker run -v` 时把 `/Users`、`/Volumes` 等转成 `/host_mnt/...`，否则 dockerd 不认路径。Linux 原生 Docker 不要设。 |
| `EXEC_MCP_LOG_CONFIG` | 可选，设为 `1` 时在启动时打印解析后的路径（排错用）。 |

**禁止**：`EXEC_HOST_REPO_DIR` 与 `MCP_EXEC_REPO_MOUNT` 写成同一个字符串（例如都填 `/octoplus`）。前者是宿主机路径，后者是容器内路径。

### 推荐命令（macOS + Docker Desktop）

将仓库路径换成你的实际路径。

```bash
docker build -t octosucker-exec-sandbox:latest \
  -f /Users/zecrey/Desktop/OctoSucker/OctoSucker/mcpsvc/exec/sandbox.Dockerfile \
  /Users/zecrey/Desktop/OctoSucker/OctoSucker

export EXEC_HOST_REPO_DIR=/Users/zecrey/Desktop/OctoSucker/OctoSucker
export MCP_EXEC_REPO_MOUNT=/octoplus
export EXEC_WORKSPACE_DIRS=/octoplus/agent/workspace
export EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX=1
export MCP_EXEC_PORT=9876

docker compose --project-directory "$EXEC_HOST_REPO_DIR" \
  -f "$EXEC_HOST_REPO_DIR/mcpsvc/exec/docker-compose.exec.yml" up --build
```

**Linux 宿主机**（无 Docker Desktop）：不要设置 `EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX`（或显式置空）。

修改变量后需重新 `docker compose up`（必要时 `--force-recreate`）。`Dockerfile.mcp-exec` 或 exec 代码变更后需带 `--build`。

### 排查

1. **`EXEC_MCP_LOG_CONFIG=1`** 重启 mcp-exec，看日志里的 `workspace_roots` / `nested_docker_bind_sources`。
2. **内联变量**（避免 shell 里历史 `export` 干扰）：把上面所有 `export` 改成同一行 `VAR=value ... docker compose ...`。
3. **项目根 `.env`**：可能与 shell 冲突；shell 变量优先于 `.env`，不确定时用内联。
4. **`docker compose ... config`**：核对 `environment` 块。
5. **旧 sandbox**：`docker rm -f octosucker-agent-sandbox`。

嵌套 bind 报 **`/repo/...` 或 `/octoplus/...` not shared**：说明当时 **`EXEC_HOST_REPO_DIR` 被设成了容器内挂载点**；与 `MCP_EXEC_REPO_MOUNT` 相同。服务只读环境变量，不会自动改错。
