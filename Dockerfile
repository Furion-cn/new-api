FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/oven/bun:latest AS builder

WORKDIR /build
COPY web/package.json .
RUN bun install
COPY ./web .
COPY ./VERSION .
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/golang:1.24-alpine AS builder2

# 安装 git（放在最前面以利用缓存）
RUN apk add --no-cache git

# 定义代理环境变量
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=https://goproxy.cn \
    GOTOOLCHAIN=auto \
    GOPRIVATE=github.com/Furion-cn/* \
    HTTP_PROXY=${HTTP_PROXY} \
    HTTPS_PROXY=${HTTPS_PROXY} \
    NO_PROXY=${NO_PROXY}

# 定义私有仓库鉴权环境变量
ARG GITHUB_USERNAME
ARG GITHUB_TOKEN
ARG GITHUB_PRIVATE_URL="https://github.com/"

WORKDIR /build

# 使用环境变量配置 git 以支持私有仓库访问
RUN if [ -n "$GITHUB_USERNAME" ] && [ -n "$GITHUB_TOKEN" ]; then \
        git config --global url."https://${GITHUB_USERNAME}:${GITHUB_TOKEN}@github.com/".insteadOf "${GITHUB_PRIVATE_URL}"; \
        git config --global url."https://${GITHUB_USERNAME}:${GITHUB_TOKEN}@github.com/Furion-cn/".insteadOf "https://github.com/Furion-cn/"; \
        git config --global credential.helper store; \
        echo "https://${GITHUB_USERNAME}:${GITHUB_TOKEN}@github.com" > ~/.git-credentials; \
        git config --global http.sslVerify false; \
    fi

ADD go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .
COPY --from=builder /build/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && \
    go build -ldflags "-s -w -X 'one-api/common.Version=$(cat VERSION)'" -o one-api

FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/library/alpine:latest

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk update \
    && apk upgrade \
    && apk add --no-cache ca-certificates tzdata ffmpeg logrotate dcron curl neovim\
    && update-ca-certificates


    # 复制清理脚本到容器中
COPY cleanup-logs.sh /usr/local/bin/cleanup-logs.sh
RUN chmod +x /usr/local/bin/cleanup-logs.sh

# 复制logrotate配置文件
COPY logrotate.conf /etc/logrotate.d/one-api

# 创建logrotate状态文件目录
RUN mkdir -p /var/lib/logrotate

COPY --from=builder2 /build/one-api /
COPY docker-entrypoint.sh /
RUN chmod +x /docker-entrypoint.sh

# 预下载 tiktoken BPE 数据文件，避免运行时从网络下载导致启动慢
RUN mkdir -p /tiktoken_cache && \
    wget -O /tiktoken_cache/9b5ad71b2ce5302211f9c61530b329a4922fc6a4 \
      "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken" && \
    wget -O /tiktoken_cache/fb374d419588a4632f3f557e76b4b70aebbca790 \
      "https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken"
ENV TIKTOKEN_CACHE_DIR=/tiktoken_cache

EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/docker-entrypoint.sh"]