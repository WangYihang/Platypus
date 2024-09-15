# Stage 1: Builder
FROM golang:1.22.6-alpine AS builder

# Install necessary packages and tools
RUN apk add --no-cache git upx \
    && go install github.com/goreleaser/goreleaser/v2@latest \
    && go install github.com/air-verse/air@latest \
    && go install golang.org/x/tools/cmd/goimports@latest \
    && go install github.com/fzipp/gocyclo/cmd/gocyclo@latest \
    && go install github.com/go-critic/go-critic/cmd/gocritic@latest \
    && go install github.com/BurntSushi/toml/cmd/tomlv@latest \
    && go get -u github.com/go-bindata/go-bindata/...

# Set up the working directory
WORKDIR /app

# Copy source code
COPY . .

# Check if the current commit is tagged, and build accordingly
RUN if git describe --tags --exact-match >/dev/null 2>&1; then \
      echo "Commit is tagged. Creating a release build."; \
      goreleaser build --clean; \
    else \
      echo "Commit is not tagged. Creating a snapshot build."; \
      goreleaser build --clean --snapshot; \
    fi

# Stage 2: Final image
FROM ubuntu:24.04

# Copy necessary binaries from the builder stage
COPY --from=builder /go/bin/goreleaser /usr/local/bin/goreleaser
COPY --from=builder /go/bin/air /usr/local/bin/air
COPY --from=builder /go/bin/goimports /usr/local/bin/goimports
COPY --from=builder /go/bin/gocyclo /usr/local/bin/gocyclo
COPY --from=builder /go/bin/gocritic /usr/local/bin/gocritic
COPY --from=builder /app/dist/platypus_linux_amd64_v1/platypus /usr/local/bin/platypus
