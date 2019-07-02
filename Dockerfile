FROM golang:1.12.6-alpine as builder

ENV GOPROXY https://goproxy.io

WORKDIR /Platypus

COPY . /Platypus

RUN go build -o platypus platypus.go

FROM alpine:3.9

COPY --from=builder /Platypus/platypus /bin/platypus

ENTRYPOINT ["sh", "-c", "/bin/platypus"]
