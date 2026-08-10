## `/srv/seedao` 目录模板（本地生成）

这份目录用于在新服务器上复刻旧服务器的 `/srv/seedao` 结构，便于直接 `scp/rsync` 上去后 `docker compose up -d`。

### 目录内容

- `docker-compose.yml`：生产主文件（与旧服务器一致的服务定义）
- `docker-compose.prod.yml`：生产文件（旧服务器上与 `docker-compose.yml` 内容相同）
- `docker-compose.staging.yml`：staging 文件（旧服务器版本）
- `sync_with_prod.sh`：旧服务器的同步脚本模板（已将远端主机改为占位符）
- `conf/`：按服务划分的配置目录（只放模板/占位，**不要提交密钥**）
- `logs/`：日志目录（仅目录结构）

### 镜像仓库（GHCR）

所有服务镜像统一使用组织 **`ghcr.io/seedao-polis/`**（旧名 `ghcr.io/seedao-devops/`、`ghcr.io/taoist-labs/` 已废弃）。

若服务器上 `/srv/seedao/docker-compose.yml` 仍为旧组织名，在服务器执行：

```bash
cd /srv/seedao
sed -i.bak 's|ghcr.io/seedao-devops/|ghcr.io/seedao-polis/|g; s|ghcr.io/taoist-labs/|ghcr.io/seedao-polis/|g' docker-compose.yml
grep 'image:' docker-compose.yml
docker compose pull os-backend   # 或按需 pull 其他服务
docker compose up -d
```

拉取前需已登录 GHCR（对 `seedao-polis` 包有读权限）：

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <github_user> --password-stdin
```

### 使用方式（在新服务器）

1. 将本目录内容同步到新服务器 `/srv/seedao/`
2. 在新服务器补齐（或替换）配置文件，例如：
   - `conf/os-backend/config.yml.prod`
3. 创建/确认日志目录存在，例如：
   - `logs/os-backend/`
4. 启动：

```bash
cd /srv/seedao
docker compose -f docker-compose.yml up -d
```

### 安全提醒

- 本目录中的所有密钥/Token 已被替换为占位符。你需要在服务器上填入真实值，并确保权限收紧（例如 `chmod 600`）。
- 不要把服务器的 `conf/*/*.prod` 明文配置提交到仓库。

