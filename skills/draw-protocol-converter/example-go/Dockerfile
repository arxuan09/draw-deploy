# 协议转换器镜像（可选；不用 Docker 的话直接 go build 出可执行文件，和 .env 放一起运行）。
# 多阶段构建出静态二进制，配置用 .env 传进去：
#
#   docker build -t my-converter .
#   docker run -d --restart unless-stopped -p 8787:8787 --env-file .env my-converter
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /converter .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /converter /converter
ENV PORT=8787
EXPOSE 8787
USER nobody
ENTRYPOINT ["/converter"]
