# Stage 1: Build the binaries
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-server ./cmd/platypus-server

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-agent ./cmd/platypus-agent

# Stage 2: Server runtime
FROM gcr.io/distroless/static-debian12:nonroot AS server
COPY --from=builder /out/platypus-server /usr/local/bin/platypus-server
USER nonroot:nonroot
EXPOSE 7331 13337
ENTRYPOINT ["/usr/local/bin/platypus-server"]

# Stage 3: Agent runtime
FROM gcr.io/distroless/static-debian12:nonroot AS agent
COPY --from=builder /out/platypus-agent /usr/local/bin/platypus-agent
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/platypus-agent"]
