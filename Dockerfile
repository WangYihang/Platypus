# Stage 1: Builder
FROM golang:1.23.2 AS builder

# # replace shell with bash so we can source files
RUN rm /bin/sh && ln -s /bin/bash /bin/sh

# Install necessary golang packages and tools
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go install github.com/goreleaser/goreleaser/v2@latest
RUN go install github.com/air-verse/air@latest
RUN go install golang.org/x/tools/cmd/goimports@latest
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
RUN go install github.com/go-critic/go-critic/cmd/gocritic@latest
RUN go install github.com/BurntSushi/toml/cmd/tomlv@latest

# Installs nvm (Node Version Manager)
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh | bash

# Download and install Node.js (you may need to restart the terminal)
RUN source ~/.nvm/nvm.sh \
    && nvm install 20 \
    && nvm alias default 20 \
    && nvm use default \
    && npm config set registry https://registry.npmmirror.com/ \
    && npm install -g yarn

# Set up the working directory
WORKDIR /app

# Copy source code
COPY . .

# Download golang dependencies
RUN /usr/local/go/bin/go mod download

# Download web dependencies
RUN source ~/.nvm/nvm.sh \
    && cd web/platypus \
    && yarn install \
    && yarn build

# Build the application
RUN goreleaser build --snapshot --clean --single-target