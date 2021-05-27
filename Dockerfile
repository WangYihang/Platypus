# Stage 1: Prepare source code
FROM alpine/git as source
WORKDIR /app
COPY .git .git
RUN git checkout .

# Stage 2: Build frontend
FROM node:14 as frontend
COPY --from=source /app/html /app/html
# Change yarn registry to fit in the networking situation in China
RUN yarn config set registry https://registry.npm.taobao.org
RUN cd /app/html/frontend && rm -rf node_modules && yarn install && yarn build
RUN cd /app/html/ttyd && rm -rf node_modules && yarn install && yarn build

# Stage 3: Build platypus
FROM golang as platypus
COPY --from=source /app /app
WORKDIR /app
COPY --from=frontend /app/html/frontend/build /app/html/frontend/build
COPY --from=frontend /app/html/ttyd/dist /app/html/ttyd/dist
RUN apt update
RUN apt install -y go-bindata
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go build -o ./termites/termite_linux_amd64 termite.go
RUN go-bindata -pkg resource -o ./lib/util/resource/resource.go ./termites/... ./lib/runtime/... ./html/ttyd/dist/... ./html/frontend/build/...
RUN go build -o ./build/platypus platypus.go

# Stage 4: running environment from scratch
FROM ubuntu
LABEL maintainer="Wang Yihang <wangyihanger@gmail.com>"
COPY --from=platypus /app/build/platypus /app/platypus
RUN apt update
RUN apt install -y tmux upx
WORKDIR /app
RUN echo "setw -g aggressive-resize on" > .tmux.conf
ENTRYPOINT tmux new -s platypus ./platypus