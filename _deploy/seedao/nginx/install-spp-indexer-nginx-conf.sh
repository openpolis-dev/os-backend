#!/usr/bin/env bash
# 一键写入 spp-indexer.seedao.top 的 Nginx 站点配置（与 spp-indexer.seedao.top.conf.example 中 server 块一致）。
#
# 用法（在服务器上，于本脚本所在目录或给出路径）：
#   sudo bash install-spp-indexer-nginx-conf.sh
#   sudo bash install-spp-indexer-nginx-conf.sh --reload   # 写入后执行 nginx -t 并重载
#
# 指定输出路径：
#   sudo bash install-spp-indexer-nginx-conf.sh -o /etc/nginx/conf.d/spp-indexer.seedao.top.conf

set -euo pipefail

OUT="/etc/nginx/conf.d/spp-indexer.seedao.top.conf"
RELOAD=false

while [[ $# -gt 0 ]]; do
	case "$1" in
	--reload) RELOAD=true; shift ;;
	-o | --output)
		OUT="${2:?缺少 -o 参数值}"
		shift 2
		;;
	-h | --help)
		sed -n '1,12p' "$0" | tail -n +2
		exit 0
		;;
	*)
		echo "未知参数: $1（可用 --help）" >&2
		exit 1
		;;
	esac
done

if [[ "${EUID:-0}" -ne 0 ]]; then
	echo "请使用 root 或 sudo 执行（需写入 ${OUT}）。" >&2
	exit 1
fi

umask 022
tee "$OUT" >/dev/null <<'EOF'
server {
    listen 80;
    server_name spp-indexer.seedao.top;

    location ^~ /.well-known/acme-challenge/ {
        root /var/www/_acme-challenge;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    http2 on;
    server_name spp-indexer.seedao.top;

    ssl_certificate     /etc/nginx/ssl/spp-indexer.seedao.top/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/spp-indexer.seedao.top/privkey.pem;

    ssl_session_timeout 10m;
    ssl_session_cache shared:SSL:10m;

    location ^~ /sns/ {
        proxy_pass http://127.0.0.1:9092;
        proxy_http_version 1.1;
        proxy_set_header Host sns-api.seedao.top;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        proxy_pass http://127.0.0.1:9094;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

echo "已写入: ${OUT}"

if [[ "$RELOAD" == true ]]; then
	nginx -t
	systemctl reload nginx
	echo "已执行: nginx -t && systemctl reload nginx"
else
	echo "稍后请执行: nginx -t && systemctl reload nginx"
fi
