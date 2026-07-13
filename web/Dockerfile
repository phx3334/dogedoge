# 构建阶段：基于 Node 编译前端
FROM node:20-alpine AS build
WORKDIR /app

# 先装依赖（利用层缓存，仅 package 文件变更时才重装）
COPY package*.json ./
RUN npm ci

# 再拷贝源码并构建
COPY . .
RUN npm run build

# 运行阶段：nginx 托管静态资源并反代后端 API
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
