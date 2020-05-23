FROM golang:latest as builder
COPY . /Platypus
WORKDIR /Platypus
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go build -ldflags "-s -w" -o platypus platypus.go
ENTRYPOINT ["sh", "-c", "/Platypus/platypus"]
