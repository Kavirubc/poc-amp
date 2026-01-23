# Build stage for eBPF agent
FROM golang:1.21-bookworm AS builder

# Install eBPF build dependencies
RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-libc-dev \
    make \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy BPF source code
COPY bpf/ ./bpf/

# Copy Go source code
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/

# Generate eBPF bindings
WORKDIR /app/pkg/ebpf
RUN go generate ./...

# Build the agent
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /ebpf-agent ./cmd/main.go

# Runtime stage - minimal image
FROM debian:bookworm-slim

# Install runtime dependencies (just ca-certificates for HTTPS)
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary
COPY --from=builder /ebpf-agent /app/ebpf-agent

# The agent needs privileged mode to:
# - Load eBPF programs into the kernel
# - Attach to network interfaces
# - Access /sys/fs/bpf for maps
# - Access /sys/kernel/debug for tracing

ENTRYPOINT ["/app/ebpf-agent"]
CMD ["-amp-url", "http://backend:8080", "-log-level", "info"]
