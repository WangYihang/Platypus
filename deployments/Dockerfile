# Stage 1: Prepare source code
FROM alpine/git as source
WORKDIR /app
COPY ../.git .git
RUN git checkout .

# Stage 2: Build frontend
FROM node:14 as frontend
COPY --from=source /app/web /app/web
# Change yarn registry to fit in the networking situation in China
RUN yarn config set registry https://registry.npmmirror.com
RUN cd /app/web/frontend && rm -rf node_modules && yarn install && yarn build
RUN cd /app/web/ttyd && rm -rf node_modules && yarn install && yarn build

# Stage 3: Build platypus
FROM golang as builder
COPY --from=source /app /app
WORKDIR /app
COPY --from=frontend /app/web/frontend/build /app/web/frontend/build
COPY --from=frontend /app/web/ttyd/dist /app/web/ttyd/dist
RUN apt update
RUN apt install -y go-bindata
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go build -ldflags="-s -w " -trimpath -o ./build/termite/termite_linux_amd64 ./cmd/termite/main.go
RUN go-bindata -pkg assets -o ./internal/util/assets/assets.go ./assets/config.example.yml ./assets/template/rsh/... ./web/ttyd/dist/... ./web/frontend/build/... ./build/termite/...
RUN go build -ldflags="-s -w " -trimpath -o ./build/platypus/platypus ./cmd/platypus/main.go

# Stage 4: running environment from scratch
FROM ubuntu
LABEL maintainer="Wang Yihang <wangyihanger@gmail.com>"
COPY --from=builder /app/build/platypus/platypus /app/platypus
RUN apt update
RUN apt install -y tmux upx
WORKDIR /app
RUN echo "setw -g aggressive-resize on" > /root/.tmux.conf
