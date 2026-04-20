# Stage 1: Build the server binary
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/platypus-server ./cmd/platypus-server

# Stage 2: Minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/platypus-server /usr/local/bin/platypus-server

USER nonroot:nonroot
EXPOSE 7331 13337
ENTRYPOINT ["/usr/local/bin/platypus-server"]
