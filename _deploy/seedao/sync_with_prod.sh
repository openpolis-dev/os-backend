#!/bin/bash

set -euo pipefail

# 旧服务器脚本的本地模板。
# 将当前目录同步到远端服务器的 /srv/seedao（或你的目标目录）。
# 使用前请把 <remote_host> 替换为你的 SSH Host（例如 ~/.ssh/config 里的 Host 名），或直接替换为 user@ip。

rsync -auv --progress \
  --exclude='node_modules' \
  --exclude='venv' \
  --exclude='.venv' \
  --exclude='win_venv' \
  --exclude='wsl_venv' \
  --exclude='.git' \
  . seedao1:/srv/seedao

