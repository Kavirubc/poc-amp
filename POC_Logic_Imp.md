# POC: AI Agent Management Platform with eBPF-Enhanced Compensation

## Executive Summary

This Proof of Concept (POC) demonstrates a comprehensive AI Agent Management Platform that combines:
1. **Container-based Agent Deployment** - Docker-managed AI agent lifecycle
2. **LLM-Suggested Compensation Mappings** - Automatic rollback capability with human approval
3. **eBPF-based Observability** - Kernel-level monitoring for agent behavior verification

The goal is to provide a secure, observable, and recoverable environment for AI agents operating with real-world tools.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Component Deep Dive](#component-deep-dive)
3. [Compensation System](#compensation-system)
4. [eBPF Integration](#ebpf-integration)
5. [Data Flow](#data-flow)
6. [API Reference](#api-reference)
7. [Testing the POC](#testing-the-poc)
8. [Future Enhancements](#future-enhancements)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Host System                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Docker Compose Stack                              │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │   │
│  │  │   Frontend   │  │   Backend    │  │  PostgreSQL  │              │   │
│  │  │   Next.js    │  │     Go       │  │              │              │   │
│  │  │   :3000      │  │    :8080     │  │    :5432     │              │   │
│  │  └──────────────┘  └──────┬───────┘  └──────────────┘              │   │
│  │                           │                                         │   │
│  │                    Docker Socket Mount                              │   │
│  │                           │                                         │   │
│  │  ┌────────────────────────▼─────────────────────────────────────┐  │   │
│  │  │              Agent Containers (Sibling)                       │  │   │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │  │   │
│  │  │  │  Agent 1    │  │  Agent 2    │  │  Agent N    │          │  │   │
│  │  │  │  :9001      │  │  :9002      │  │  :900N      │          │  │   │
│  │  │  │             │  │             │  │             │          │  │   │
│  │  │  │ Labels:     │  │ Labels:     │  │ Labels:     │          │  │   │
│  │  │  │ amp.managed │  │ amp.managed │  │ amp.managed │          │  │   │
│  │  │  │ amp.id=UUID │  │ amp.id=UUID │  │ amp.id=UUID │          │  │   │
│  │  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │  │   │
│  │  └─────────┼────────────────┼────────────────┼──────────────────┘  │   │
│  └────────────┼────────────────┼────────────────┼──────────────────────┘   │
│               │                │                │                           │
│  ┌────────────▼────────────────▼────────────────▼──────────────────────┐   │
│  │                     eBPF Monitoring Layer                            │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │   │
│  │  │   Syscall    │  │   Network    │  │    File      │              │   │
│  │  │   Tracing    │  │   Monitor    │  │   Access     │              │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘              │   │
│  │                           │                                         │   │
│  │                    Filter by Container Labels                       │   │
│  │                    (amp.managed=true)                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│                              Linux Kernel                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Design Principles

1. **Docker-out-of-Docker**: Backend mounts `/var/run/docker.sock` to create sibling containers (not nested)
2. **Label-based Targeting**: All agent containers have `amp.managed=true` and `amp.id=<uuid>` for eBPF filtering
3. **Network Isolation**: Agents run on dedicated `amp-network` bridge for traffic inspection
4. **Separation of Concerns**:
   - Frontend: UI for management and approval workflows
   - Backend: API, orchestration, and compensation logic
   - eBPF: Independent kernel-level observation

---

## Component Deep Dive

### 1. Backend (Go)

**Location**: `backend/`

**Responsibilities**:
- Agent CRUD operations
- Git repository cloning and type detection
- Docker image building and container lifecycle
- Compensation mapping management
- Transaction logging and rollback orchestration

**Key Services**:

| Service | File | Purpose |
|---------|------|---------|
| `AgentService` | `services/agent.go` | Orchestrates agent lifecycle |
| `DockerService` | `services/docker.go` | Container and image management |
| `GitService` | `services/git.go` | Repository cloning, type detection |
| `SuggestionService` | `services/suggestion.go` | Heuristic + LLM compensation suggestions |
| `RecoveryService` | `services/recovery.go` | Rollback plan generation and execution |

**Environment Injection**:
When starting agent containers, the backend automatically injects:
```
AGENT_ID=<uuid>        # For transaction logging
AMP_URL=http://backend:8080  # For API communication
PORT=8000              # Internal container port
```

### 2. Frontend (Next.js)

**Location**: `frontend/`

**Key Pages**:
- `/` - Dashboard with agent list
- `/agents/new` - Create new agent form
- `/agents/[id]` - Agent detail with tabs:
  - **Details**: Status, port, container info
  - **Logs**: Real-time container logs (SSE)
  - **Compensation**: Mapping approval workflow

### 3. Database (PostgreSQL)

**Tables**:

```sql
-- Core agent management
agents (
    id, name, repo_url, branch, type, status,
    port, container_id, image_id, env_content,
    created_at, updated_at
)

-- Compensation system
compensation_mappings (
    id, agent_id, tool_name, tool_schema, tool_description,
    compensator_name, parameter_mapping,
    status,        -- pending, approved, rejected, no_compensation
    suggested_by,  -- heuristic, llm, manual
    confidence, reasoning,
    reviewed_by, reviewed_at,
    created_at, updated_at
)

-- Transaction tracking
transaction_logs (
    id, agent_id, session_id,
    tool_name, input_params, output_result,
    status,        -- executed, compensated, failed
    executed_at, compensated_at,
    compensation_id, compensation_result,
    created_at
)
```

### 4. Python Instrumentation Library

**Location**: `amp-instrumentation/`

**Purpose**: SDK for agents to integrate with AMP's compensation system

**Components**:

```python
from amp_instrumentation import AMPInterceptor, CompensationRegistry, RecoveryManager

# Main interceptor for tool wrapping
interceptor = AMPInterceptor(agent_id="uuid", amp_url="http://backend:8080")

# Register tools for compensation analysis
interceptor.register_tools([...])

# Wrap tool functions for automatic logging
@interceptor.wrap_tool("book_flight")
def book_flight(flight_id, passenger_id):
    return {"booking_id": "BK123"}

# On failure, get rollback plan
plan = interceptor.get_rollback_plan()

# Execute rollback
result = interceptor.execute_rollback(tool_executor=lambda name, params: ...)
```

---

## Compensation System

### Overview

The compensation system implements the **SAGA pattern** for distributed transactions, allowing AI agents to rollback side effects when failures occur.

### Workflow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Compensation Workflow                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. TOOL REGISTRATION                                                   │
│     ┌──────────┐     ┌──────────────┐     ┌────────────────┐           │
│     │  Agent   │────▶│   Backend    │────▶│  Suggestion    │           │
│     │  Tools   │     │   /tools     │     │   Service      │           │
│     └──────────┘     └──────────────┘     └────────┬───────┘           │
│                                                     │                    │
│                              ┌──────────────────────┼──────────────┐    │
│                              ▼                      ▼              │    │
│                      ┌──────────────┐      ┌──────────────┐       │    │
│                      │  Heuristic   │      │     LLM      │       │    │
│                      │  Matching    │      │   (Gemini)   │       │    │
│                      └──────┬───────┘      └──────┬───────┘       │    │
│                             │                      │               │    │
│                             └──────────┬───────────┘               │    │
│                                        ▼                           │    │
│  2. HUMAN APPROVAL          ┌──────────────────┐                  │    │
│                             │  Pending Mapping │                  │    │
│     ┌──────────┐            │  - tool_name     │                  │    │
│     │  Human   │◀───────────│  - compensator   │                  │    │
│     │ Reviews  │            │  - param_mapping │                  │    │
│     └────┬─────┘            │  - confidence    │                  │    │
│          │                  └──────────────────┘                  │    │
│          ▼                                                        │    │
│   ┌────────────┐  ┌────────────┐  ┌────────────────┐             │    │
│   │  Approve   │  │   Reject   │  │ No Compensation │             │    │
│   │  (edit)    │  │            │  │   (skip)        │             │    │
│   └────────────┘  └────────────┘  └────────────────┘             │    │
│                                                                   │    │
│  3. TRANSACTION LOGGING                                           │    │
│     ┌──────────┐     ┌──────────────┐     ┌────────────────┐     │    │
│     │  Agent   │────▶│   Backend    │────▶│  Transaction   │     │    │
│     │ Executes │     │ /transactions│     │     Log        │     │    │
│     │  Tool    │     └──────────────┘     └────────────────┘     │    │
│     └──────────┘                                                  │    │
│                                                                   │    │
│  4. ROLLBACK (on failure)                                         │    │
│     ┌──────────┐     ┌──────────────┐     ┌────────────────┐     │    │
│     │ Trigger  │────▶│  Generate    │────▶│  Execute Plan  │     │    │
│     │ Rollback │     │  Plan (LIFO) │     │  (compensate/  │     │    │
│     └──────────┘     └──────────────┘     │   skip)        │     │    │
│                                           └────────────────┘     │    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Heuristic Pattern Matching

Before calling the LLM, the system tries pattern matching:

```go
var undoPrefixes = map[string]string{
    "book_":      "cancel_",
    "create_":    "cancel_",
    "make_":      "cancel_",
    "add_":       "remove_",
    "insert_":    "delete_",
    "reserve_":   "cancel_",
    "allocate_":  "deallocate_",
    "assign_":    "unassign_",
    "start_":     "stop_",
    "enable_":    "disable_",
    "open_":      "close_",
    "send_":      "recall_",
    "publish_":   "unpublish_",
    "activate_":  "deactivate_",
    "lock_":      "unlock_",
    "subscribe_": "unsubscribe_",
}
```

**Example**: `book_flight` → looks for `cancel_flight` in available tools

### Parameter Mapping

Compensation parameters are extracted from original input/output:

```json
{
  "booking_id": "result.booking_id",
  "reason": "input.cancellation_reason"
}
```

**Paths**:
- `input.<field>` - From original tool input
- `result.<field>` or `output.<field>` - From original tool output

### Rollback Plan Example

```json
{
  "agent_id": "355ec692-e8bd-40be-afc6-9a8e749465a1",
  "session_id": "session-123",
  "steps": [
    {
      "transaction_id": "tx-001",
      "tool_name": "send_confirmation_email",
      "action": "skip",
      "reason": "no approved compensation mapping"
    },
    {
      "transaction_id": "tx-002",
      "tool_name": "create_reservation",
      "action": "compensate",
      "compensator_name": "cancel_reservation",
      "compensation_params": {
        "reservation_id": "RES-FULLTEST"
      }
    },
    {
      "transaction_id": "tx-003",
      "tool_name": "book_flight",
      "action": "compensate",
      "compensator_name": "cancel_flight",
      "compensation_params": {
        "booking_id": "BK-FULLTEST"
      }
    }
  ]
}
```

Note: Steps are in **LIFO order** (last transaction first).

---

## eBPF Integration

### Why eBPF?

Traditional agent monitoring relies on **self-reporting** - agents tell the platform what they're doing. This has fundamental limitations:

1. **Trust Issue**: Malicious or buggy agents may not report accurately
2. **Incomplete Coverage**: Not all side effects are captured by tool abstractions
3. **Reactive**: We only know what happened after it's reported

**eBPF solves this** by providing kernel-level visibility into actual agent behavior:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Self-Reporting vs eBPF Monitoring                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  SELF-REPORTING (Current)           eBPF MONITORING (Enhanced)          │
│  ─────────────────────────          ──────────────────────────          │
│                                                                          │
│  Agent says:                        Kernel observes:                     │
│  "I called book_flight API"         - HTTP POST to api.airline.com      │
│                                     - TLS handshake with cert           │
│                                     - 2KB request, 1KB response         │
│                                     - DNS lookup for api.airline.com    │
│                                                                          │
│  Agent says:                        Kernel observes:                     │
│  "I wrote config.json"              - open("/app/config.json", O_WRONLY)│
│                                     - write(fd, data, 1024)             │
│                                     - fsync(fd)                         │
│                                                                          │
│  Agent says nothing                 Kernel observes:                     │
│                                     - connect() to unknown IP           │
│                                     - execve("/bin/sh", ...)            │
│                                     - ptrace() attempt                  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### eBPF Programs for Agent Monitoring

#### 1. Syscall Tracing

Monitor system calls from agent containers:

```c
// Pseudo-code for eBPF syscall tracer
SEC("tracepoint/syscalls/sys_enter_openat")
int trace_openat(struct trace_event_raw_sys_enter *ctx) {
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    // Check if PID belongs to amp-managed container
    if (!is_amp_container(pid))
        return 0;

    char filename[256];
    bpf_probe_read_user_str(filename, sizeof(filename),
                            (void *)ctx->args[1]);

    // Log file access event
    struct event_t event = {
        .pid = pid,
        .type = EVENT_FILE_OPEN,
    };
    bpf_probe_read_str(event.filename, sizeof(event.filename), filename);

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU,
                          &event, sizeof(event));
    return 0;
}
```

**Tracked syscalls**:
- `openat`, `read`, `write` - File operations
- `connect`, `sendto`, `recvfrom` - Network operations
- `execve`, `clone` - Process operations
- `unlink`, `rename` - File modifications

#### 2. Network Traffic Monitor

Track all network connections from agents:

```c
SEC("kprobe/tcp_connect")
int trace_tcp_connect(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    if (!is_amp_container(pid))
        return 0;

    struct event_t event = {
        .pid = pid,
        .type = EVENT_TCP_CONNECT,
    };

    // Extract destination IP and port
    bpf_probe_read(&event.daddr, sizeof(event.daddr), &sk->__sk_common.skc_daddr);
    bpf_probe_read(&event.dport, sizeof(event.dport), &sk->__sk_common.skc_dport);

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU,
                          &event, sizeof(event));
    return 0;
}
```

#### 3. Container Identification

Link PIDs to agent containers using cgroup:

```c
// Map container ID to agent metadata
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u64);    // cgroup_id
    __type(value, struct agent_info);
} agent_map SEC(".maps");

static __always_inline bool is_amp_container(u32 pid) {
    u64 cgroup_id = bpf_get_current_cgroup_id();
    struct agent_info *info = bpf_map_lookup_elem(&agent_map, &cgroup_id);
    return info != NULL;
}
```

### eBPF + Compensation Integration

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  eBPF-Enhanced Compensation Flow                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌────────────────┐                                                     │
│  │  Agent calls   │                                                     │
│  │  book_flight() │                                                     │
│  └───────┬────────┘                                                     │
│          │                                                               │
│          ▼                                                               │
│  ┌────────────────┐     ┌────────────────┐                             │
│  │ Agent reports  │     │  eBPF observes │                             │
│  │ to AMP API     │     │  actual calls  │                             │
│  └───────┬────────┘     └───────┬────────┘                             │
│          │                       │                                       │
│          │  Transaction Log      │  eBPF Events                         │
│          │  - tool: book_flight  │  - TCP connect: api.airline.com:443  │
│          │  - input: {...}       │  - TLS handshake                     │
│          │  - output: {...}      │  - HTTP request/response             │
│          │                       │  - DNS: api.airline.com              │
│          ▼                       ▼                                       │
│  ┌─────────────────────────────────────────┐                           │
│  │           Correlation Engine            │                           │
│  │  - Match reported tool to observed I/O  │                           │
│  │  - Detect unreported side effects       │                           │
│  │  - Identify anomalies                   │                           │
│  └───────────────────┬─────────────────────┘                           │
│                      │                                                   │
│          ┌───────────┴───────────┐                                      │
│          ▼                       ▼                                       │
│  ┌──────────────┐       ┌──────────────┐                               │
│  │   Normal     │       │   Anomaly    │                               │
│  │  Execution   │       │  Detected    │                               │
│  └──────────────┘       └──────┬───────┘                               │
│                                 │                                        │
│                                 ▼                                        │
│                        ┌──────────────┐                                 │
│                        │   Trigger    │                                 │
│                        │  Automatic   │                                 │
│                        │   Rollback   │                                 │
│                        └──────────────┘                                 │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Use Cases for eBPF in AMP

| Use Case | eBPF Capability | Compensation Benefit |
|----------|-----------------|---------------------|
| **Verify API calls** | Track outbound HTTP/HTTPS | Confirm reported tool execution matches actual network calls |
| **Detect data exfiltration** | Monitor egress traffic | Alert/block unauthorized data transfers, trigger rollback |
| **Track file modifications** | Trace file syscalls | Know exactly what files changed for precise compensation |
| **Catch unreported actions** | Full syscall visibility | Detect tools that have side effects not captured in schema |
| **Resource usage** | CPU/memory tracking | Terminate runaway agents before damage |
| **Security violations** | Detect shell escapes | Immediate container termination and rollback |

### Container Labels for eBPF Filtering

Every agent container is labeled for easy eBPF targeting:

```yaml
labels:
  amp.managed: "true"      # Identifies AMP-managed containers
  amp.agent: "agent-name"  # Human-readable name
  amp.id: "uuid"           # Unique identifier for correlation
```

The eBPF programs use these labels (via cgroup mapping) to filter events to only AMP-managed agents.

### Implementation Approach

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    eBPF Implementation Stack                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      User Space (Go)                             │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │   │
│  │  │   Backend   │  │  eBPF Event │  │  Correlation Engine     │ │   │
│  │  │   (API)     │◀─│  Consumer   │◀─│  (match events to       │ │   │
│  │  │             │  │             │  │   transactions)         │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                              ▲                                          │
│                              │ perf buffer / ring buffer                │
│                              │                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                     Kernel Space (eBPF)                          │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │   │
│  │  │  Syscall    │  │  Network    │  │  Container ID           │ │   │
│  │  │  Tracers    │  │  Hooks      │  │  Mapping                │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  Libraries: cilium/ebpf (Go), libbpf, BCC                               │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Data Flow

### Complete Request Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        End-to-End Data Flow                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. AGENT DEPLOYMENT                                                     │
│                                                                          │
│     User ──▶ Frontend ──▶ Backend ──▶ Git Clone ──▶ Docker Build        │
│                              │                          │                │
│                              │                          ▼                │
│                              │                    Docker Run             │
│                              │                    (with labels)          │
│                              │                          │                │
│                              ▼                          ▼                │
│                         PostgreSQL              Agent Container          │
│                         (agents)                 (amp-network)           │
│                                                                          │
│  2. TOOL REGISTRATION                                                    │
│                                                                          │
│     Agent ──▶ POST /agents/{id}/tools ──▶ SuggestionService             │
│                                                │                         │
│                              ┌─────────────────┴─────────────────┐      │
│                              ▼                                   ▼      │
│                        Heuristic                              LLM       │
│                        Matching                            (Gemini)     │
│                              │                                   │      │
│                              └─────────────────┬─────────────────┘      │
│                                                ▼                         │
│                                          PostgreSQL                      │
│                                    (compensation_mappings)               │
│                                                                          │
│  3. TOOL EXECUTION                                                       │
│                                                                          │
│     Agent ──▶ Execute Tool ──▶ POST /transactions ──▶ PostgreSQL        │
│        │                                              (transaction_logs) │
│        │                                                                 │
│        └──────────────────────────────────┐                             │
│                                           ▼                              │
│                                    eBPF Observes                         │
│                                    (syscalls, network)                   │
│                                           │                              │
│                                           ▼                              │
│                                    Event Stream ──▶ Correlation          │
│                                                                          │
│  4. ROLLBACK                                                             │
│                                                                          │
│     Trigger ──▶ GET /rollback-plan ──▶ RecoveryService                  │
│                                              │                           │
│                                              ▼                           │
│                                   Generate LIFO Plan                     │
│                                   (approved mappings only)               │
│                                              │                           │
│                                              ▼                           │
│     Agent ◀── Execute Compensators ◀── POST /rollback                   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## API Reference

### Agent Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/agents` | List all agents |
| POST | `/api/v1/agents` | Create agent |
| GET | `/api/v1/agents/:id` | Get agent details |
| DELETE | `/api/v1/agents/:id` | Delete agent |
| POST | `/api/v1/agents/:id/start` | Start agent |
| POST | `/api/v1/agents/:id/stop` | Stop agent |
| GET | `/api/v1/agents/:id/logs` | Stream logs (SSE) |

### Compensation System

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/agents/:id/tools` | Register tools for compensation analysis |
| GET | `/api/v1/agents/:id/compensation-mappings` | List all mappings |
| GET | `/api/v1/agents/:id/compensation-mappings/approved` | Get approved registry |
| POST | `/api/v1/compensation-mappings/:id/approve` | Approve mapping |
| POST | `/api/v1/compensation-mappings/:id/reject` | Reject mapping |
| PUT | `/api/v1/compensation-mappings/:id` | Update mapping |

### Transaction Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/agents/:id/transactions` | Log tool execution |
| GET | `/api/v1/agents/:id/sessions/:sessionId/rollback-plan` | Get rollback plan |
| POST | `/api/v1/agents/:id/sessions/:sessionId/rollback` | Execute rollback |

---

## Testing the POC

### Prerequisites

- Docker and Docker Compose
- Go 1.21+
- Node.js 20+

### Quick Start

```bash
# Clone and start
cd poc-amp
docker-compose up --build -d

# Access
# Frontend: http://localhost:3000
# Backend:  http://localhost:8080
```

### Test Compensation Workflow

```bash
# 1. Create travel booking agent
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "travel-agent", "repo_url": "https://github.com/Kavirubc/travel-booking-agent", "branch": "main"}'

# Save the agent ID from response
AGENT_ID="<uuid-from-response>"

# 2. Wait for deployment, then register tools
curl -X POST "http://localhost:8080/api/v1/agents/$AGENT_ID/tools" \
  -H "Content-Type: application/json" \
  -d '{"tools": [
    {"name": "book_flight", "description": "Books a flight"},
    {"name": "cancel_flight", "description": "Cancels a flight"},
    {"name": "create_reservation", "description": "Creates hotel reservation"},
    {"name": "cancel_reservation", "description": "Cancels reservation"}
  ]}'

# 3. Approve mappings (get mapping ID from list)
curl "http://localhost:8080/api/v1/agents/$AGENT_ID/compensation-mappings"

MAPPING_ID="<book_flight-mapping-id>"
curl -X POST "http://localhost:8080/api/v1/compensation-mappings/$MAPPING_ID/approve" \
  -H "Content-Type: application/json" \
  -d '{"parameter_mapping": {"booking_id": "result.booking_id"}}'

# 4. Log a transaction
curl -X POST "http://localhost:8080/api/v1/agents/$AGENT_ID/transactions" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "test-session",
    "tool_name": "book_flight",
    "input": {"flight_id": "UA-123"},
    "output": {"booking_id": "BK-789"}
  }'

# 5. Get rollback plan
curl "http://localhost:8080/api/v1/agents/$AGENT_ID/sessions/test-session/rollback-plan"

# Expected: Plan shows cancel_flight with booking_id: BK-789
```

---

## Future Enhancements

### Phase 1: eBPF Integration (Next)

1. **eBPF Agent** - Separate service for kernel-level monitoring
2. **Event Correlation** - Match eBPF events to transaction logs
3. **Anomaly Detection** - Alert on unreported side effects
4. **Auto-Rollback Triggers** - Automatic compensation on policy violations

### Phase 2: Advanced Features

1. **LLM-based Parameter Inference** - Smarter extraction of compensation params
2. **Compensation Testing** - Dry-run mode for validating mappings
3. **Multi-Agent Coordination** - Cross-agent transaction management
4. **Audit Trail** - Complete history of all compensations

### Phase 3: Production Readiness

1. **Authentication/Authorization** - RBAC for approval workflows
2. **Metrics/Monitoring** - Prometheus/Grafana integration
3. **High Availability** - Clustered backend deployment
4. **Policy Engine** - OPA integration for compensation rules

---

## References

- [SAGA Pattern](https://microservices.io/patterns/data/saga.html) - Distributed transaction compensation
- [eBPF Documentation](https://ebpf.io/what-is-ebpf/) - Kernel-level programmability
- [cilium/ebpf](https://github.com/cilium/ebpf) - Go library for eBPF
- [WSO2 AMP](https://wso2.com/) - Production agent management platform

---

## Repository Structure

```
poc-amp/
├── POC_Logic_Imp.md           # This document
├── docker-compose.yml         # Full stack orchestration
├── .env.example               # Environment template
│
├── backend/                   # Go API server
│   ├── main.go
│   ├── api/handlers/          # HTTP handlers
│   ├── services/              # Business logic
│   ├── store/                 # Database operations
│   └── models/                # Data structures
│
├── frontend/                  # Next.js UI
│   ├── app/                   # Pages
│   ├── components/            # React components
│   └── lib/                   # API client
│
├── db/migrations/             # SQL schemas
│
├── amp-instrumentation/       # Python SDK
│   └── amp_instrumentation/
│
├── example-travel-agent/      # Test agent
│
└── workspace/                 # Cloned repos (runtime)
```

---

*Document Version: 1.0*
*Last Updated: January 2026*
