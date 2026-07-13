#!/bin/sh
set -e

# 容器启动时，将镜像内置的静态种子（默认头像等）同步到已挂载的 uploads 卷。
# 使用 -n（no-clobber）避免覆盖用户已上传的文件；首次部署或文件缺失时补齐。
# 这样无需在宿主机手动 docker cp，重新部署后默认头像即可通过 /uploads/avatar/default.jpg 访问。
if [ -d /app/uploads-seed ]; then
  cp -rn /app/uploads-seed/. /app/uploads/ 2>/dev/null || true
fi

exec /app/api "$@"
