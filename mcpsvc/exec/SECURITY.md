# mcp-exec Security Baseline (Container Runtime)

This service executes agent commands. Treat it as high risk by default.

## Required baseline

- `mcp-exec` always runs commands inside a long-lived sandbox container (`EXEC_CONTAINER_NAME`, default `octosucker-agent-sandbox`), via `docker exec`.
- Restrict writable mounts to workspace roots only (`EXEC_WORKSPACE_DIRS` under the compose mount).
- Compose: set **`EXEC_HOST_REPO_DIR`**, **`MCP_EXEC_REPO_MOUNT`**, **`EXEC_WORKSPACE_DIRS`** explicitly; nested `docker -v` bind sources are derived from `EXEC_HOST_REPO_DIR`. On Docker Desktop (Mac), set **`EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX=1`** so nested binds use `/host_mnt/...` paths the VM expects.
- Keep root filesystem read-only in runtime container: `EXEC_CONTAINER_READONLY_ROOT=1`
- Run as non-root user: `EXEC_CONTAINER_USER=65532:65532`
- Drop Linux capabilities and prevent privilege escalation (`docker run --cap-drop=ALL --security-opt no-new-privileges`)
- Keep timeout enabled (`EXEC_COMMAND_TIMEOUT_SEC`)

## Network policy

This setup intentionally keeps network enabled (`--network bridge`) so tools like `browser-use` can access internet.

Recommended hardening while keeping network:

- Use egress firewall / proxy allowlist outside container runtime
- Keep secret env vars minimal and scoped per command
- Keep command blacklist for known dangerous patterns

## Prohibited mounts (nested sandbox)

Do not extend the **nested** `docker run` sandbox with extra `-v` binds to: full host home, `~/.ssh`, cloud credential dirs, browser profiles, or `docker.sock`. The **mcp-exec** compose service intentionally bind-mounts only the repo root (host path in `EXEC_HOST_REPO_DIR`) plus the Docker socket for orchestration.
