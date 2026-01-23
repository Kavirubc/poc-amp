# eBPF Agent for AMP

This agent provides **zero-instrumentation** HTTP traffic capture for AI agents running in Docker containers. It uses eBPF (Extended Berkeley Packet Filter) to intercept network traffic at the kernel level without requiring any code changes to the agents themselves.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     eBPF Agent                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ Container    │  │ HTTP Parser  │  │ LLM Classifier       │  │
│  │ Tracker      │  │              │  │ (Gemini)             │  │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘  │
│         │                 │                     │              │
│         │    ┌────────────┴────────────┐        │              │
│         └────►      eBPF Loader        ├────────┘              │
│              └────────────┬────────────┘                       │
│                           │                                    │
└───────────────────────────┼────────────────────────────────────┘
                            │ Kernel Space
────────────────────────────┼────────────────────────────────────
                            │
┌───────────────────────────▼────────────────────────────────────┐
│  eBPF Programs (Kernel)                                        │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────────┐  │
│  │ TC Hook       │  │ tcp_sendmsg   │  │ tcp_recvmsg       │  │
│  │ (egress)      │  │ kprobe        │  │ kprobe            │  │
│  └───────────────┘  └───────────────┘  └───────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

## How It Works

1. **Container Tracking**: Watches Docker for containers with `amp.managed=true` label
2. **Traffic Capture**: Uses eBPF TC hooks and kprobes to capture HTTP traffic
3. **HTTP Parsing**: Parses raw TCP data into HTTP requests and responses
4. **Tool Classification**: Uses heuristics and optionally LLM (Gemini) to classify API calls
5. **Compensation Discovery**: Automatically discovers compensation patterns from traffic history
6. **Backend Reporting**: Reports all findings to the AMP backend for logging and approval

## Requirements

- Linux kernel 5.4+ with eBPF support
- Docker (for container tracking)
- Privileged mode (for loading eBPF programs)
- clang/llvm (for compiling eBPF programs during build)

## Building

```bash
# Generate eBPF bindings (requires Linux with clang/llvm)
go generate ./...

# Build the agent
go build -o ebpf-agent ./cmd/main.go
```

## Running

```bash
# Run with default settings
./ebpf-agent

# Run with custom AMP backend URL
./ebpf-agent -amp-url http://backend:8080

# Run with specific network interface
./ebpf-agent -iface docker0

# Run with debug logging
./ebpf-agent -log-level debug
```

## Docker Compose

The eBPF agent is included in the main docker-compose.yml with the required privileges:

```yaml
ebpf-agent:
  build:
    context: ./ebpf-agent
  privileged: true
  pid: host
  network_mode: host
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock
    - /sys/fs/bpf:/sys/fs/bpf
    - /sys/kernel/debug:/sys/kernel/debug
  cap_add:
    - SYS_ADMIN
    - NET_ADMIN
    - SYS_PTRACE
```

## What Gets Captured

The agent captures:
- **HTTP Requests**: Method, URL, headers, body
- **HTTP Responses**: Status code, headers, body
- **Container Context**: Which agent made the request
- **Timing**: Request/response timestamps

## Tool Classification

The agent automatically classifies captured API calls:

1. **Heuristic Patterns**: Common patterns like `/book`, `/reserve`, `/create` are matched
2. **Traffic History**: Looks for compensation patterns in recent traffic
3. **LLM Analysis**: Uses Gemini to analyze complex cases (requires GEMINI_API_KEY)

## Discovered Compensations

When the agent detects a potential compensation pattern, it:

1. Registers the discovery with the AMP backend
2. The mapping appears in the UI as "pending approval"
3. Operators can approve or reject the suggested compensation
4. Approved mappings are used for rollback operations

## Limitations

- Only captures unencrypted HTTP traffic (not HTTPS)
- Requires privileged mode which may not be available in all environments
- Traffic capture adds minimal overhead but may impact very high-throughput systems
- LLM classification requires external API key and network access

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GEMINI_API_KEY` | API key for Gemini LLM classification | (disabled) |

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-amp-url` | AMP backend URL | `http://localhost:8080` |
| `-iface` | Network interface to attach to | auto-detect docker0/br-* |
| `-log-level` | Log level (debug, info, warn, error) | `info` |
