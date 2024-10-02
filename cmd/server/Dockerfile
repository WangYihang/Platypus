FROM golang:1.22.6 AS builder

ARG TARGETOS
ARG TARGETARCH

RUN go install github.com/goreleaser/goreleaser/v2@v2.2.0

WORKDIR /app
COPY . .

# Check if the current commit is tagged, and build accordingly
RUN if git describe --tags --exact-match >/dev/null 2>&1; then \
      echo "Commit is tagged. Creating a release build."; \
      goreleaser build --id platypus-server --clean; \
    else \
      echo "Commit is not tagged. Creating a snapshot build."; \
      goreleaser build --id platypus-server --clean --snapshot; \
    fi

RUN cp dist/platypus-server_${TARGETOS}_${TARGETARCH}_v1/platypus-server /usr/local/bin/platypus-server

FROM alpine:3.12
RUN apk add --no-cache ca-certificates
COPY --from=builder /usr/local/bin/platypus-server /usr/local/bin/platypus-server
ENTRYPOINT [ "/usr/local/bin/platypus-server" ]