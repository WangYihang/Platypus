# Dockerfile References: https://docs.docker.com/engine/reference/builder/

# Start from the latest ubuntu base image
FROM ubuntu:latest as builder

# Add Maintainer Info
LABEL maintainer="Wang Yihang <wangyihanger@gmail.com>"

# Make tzdata happy
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai

# Install dependencies
RUN sed -i "s/archive.ubuntu.com/mirrors.ustc.edu.cn/g" /etc/apt/sources.list \
    && sed -i "s/security.ubuntu.com/mirrors.ustc.edu.cn/g" /etc/apt/sources.list \
    && apt update \
    && apt install -y apt-utils sudo make tzdata git

WORKDIR /platypus

# Copy source code
COPY .git .git
RUN git checkout .

# Build the Go app
RUN make

# Start a new stage from scratch
FROM alpine:latest

# Install dependencies
RUN apk --no-cache add ca-certificates tmux

WORKDIR /root

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /platypus/build/platypus /root/platypus

# Define default command for `run`
CMD tmux new-session -s platypus /root/platypus