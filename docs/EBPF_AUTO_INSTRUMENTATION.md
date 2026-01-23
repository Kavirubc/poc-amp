# eBPF-Based Automatic Instrumentation

## The Problem

Current approach requires agents to:
```python
# Agent developer must add this code
interceptor = AMPInterceptor(agent_id, amp_url)
interceptor.register_tools([...])

@interceptor.wrap_tool("book_flight")
def book_flight(...):
    ...
```

**This won't work in production because:**
1. Developers won't modify their existing agents
2. Different frameworks (LangChain, CrewAI, AutoGen) have different patterns
3. We can't trust agents to self-report accurately
4. Malicious agents can lie about what they're doing

## The Solution: eBPF-Based Zero-Instrumentation

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Zero-Instrumentation Architecture                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  BEFORE (Requires Code Changes)        AFTER (eBPF - No Changes Needed)     │
│  ───────────────────────────────       ─────────────────────────────────    │
│                                                                              │
│  ┌─────────────┐                       ┌─────────────┐                      │
│  │   Agent     │                       │   Agent     │  (unchanged)         │
│  │   Code      │                       │   Code      │                      │
│  │             │                       │             │                      │
│  │ @wrap_tool  │ ◄── Required         │ def book(): │  No decorators       │
│  │ def book(): │                       │   api.call()│  No SDK imports      │
│  │   ...       │                       │             │                      │
│  └──────┬──────┘                       └──────┬──────┘                      │
│         │                                     │                              │
│         │ SDK calls AMP                       │ Normal HTTP call            │
│         ▼                                     ▼                              │
│  ┌─────────────┐                       ┌─────────────────────────────┐      │
│  │    AMP      │                       │        eBPF Layer           │      │
│  │   Backend   │                       │  ┌─────────────────────┐   │      │
│  └─────────────┘                       │  │ Intercept HTTP/TCP  │   │      │
│                                        │  │ Parse tool calls    │   │      │
│                                        │  │ Extract params      │   │      │
│                                        │  │ Log automatically   │   │      │
│                                        │  └──────────┬──────────┘   │      │
│                                        └─────────────┼───────────────┘      │
│                                                      │                       │
│                                                      ▼                       │
│                                               ┌─────────────┐               │
│                                               │    AMP      │               │
│                                               │   Backend   │               │
│                                               └─────────────┘               │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## How It Works

### 1. HTTP Traffic Interception

eBPF hooks into the kernel to capture all HTTP requests from agent containers:

```c
// eBPF program to intercept HTTP requests
SEC("socket/http_filter")
int http_filter(struct __sk_buff *skb) {
    // Only process packets from AMP-managed containers
    if (!is_amp_container(skb))
        return TC_ACT_OK;

    // Extract HTTP request
    struct http_request req;
    if (parse_http_request(skb, &req) < 0)
        return TC_ACT_OK;

    // Send to userspace for analysis
    bpf_perf_event_output(skb, &events, BPF_F_CURRENT_CPU, &req, sizeof(req));

    return TC_ACT_OK;
}
```

### 2. Automatic Tool Detection

The system analyzes HTTP traffic to identify tool calls:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Tool Detection from HTTP Traffic                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Observed HTTP Request:                                                      │
│  ─────────────────────                                                      │
│  POST /api/flights/book HTTP/1.1                                            │
│  Host: api.airline.com                                                      │
│  Content-Type: application/json                                             │
│                                                                              │
│  {                                                                           │
│    "flight_id": "UA-123",                                                   │
│    "passenger": "John Doe",                                                 │
│    "class": "business"                                                      │
│  }                                                                           │
│                                                                              │
│  Observed HTTP Response:                                                     │
│  ──────────────────────                                                     │
│  HTTP/1.1 200 OK                                                            │
│  {                                                                           │
│    "booking_id": "BK-789",                                                  │
│    "status": "confirmed"                                                    │
│  }                                                                           │
│                                                                              │
│                              ▼                                               │
│                                                                              │
│  Inferred Tool Schema:                                                       │
│  ─────────────────────                                                      │
│  {                                                                           │
│    "name": "airline_book_flight",                                           │
│    "endpoint": "POST api.airline.com/api/flights/book",                     │
│    "input_schema": {                                                        │
│      "flight_id": "string",                                                 │
│      "passenger": "string",                                                 │
│      "class": "string"                                                      │
│    },                                                                        │
│    "output_schema": {                                                       │
│      "booking_id": "string",                                                │
│      "status": "string"                                                     │
│    }                                                                         │
│  }                                                                           │
│                                                                              │
│  Auto-logged Transaction:                                                    │
│  ────────────────────────                                                   │
│  {                                                                           │
│    "tool": "airline_book_flight",                                           │
│    "input": {"flight_id": "UA-123", "passenger": "John Doe"},              │
│    "output": {"booking_id": "BK-789", "status": "confirmed"}               │
│  }                                                                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3. Compensation Discovery

The system observes API patterns to suggest compensations:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Automatic Compensation Discovery                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Observed Traffic Pattern:                                                   │
│                                                                              │
│  Request 1: POST api.airline.com/api/flights/book                           │
│             → Response: {booking_id: "BK-789"}                              │
│                                                                              │
│  Request 2: DELETE api.airline.com/api/flights/BK-789                       │
│             → Response: {status: "cancelled"}                               │
│                                                                              │
│                              ▼                                               │
│                                                                              │
│  LLM Analysis:                                                               │
│  "POST /flights/book creates a booking, DELETE /flights/{id} cancels it.   │
│   The booking_id from the POST response is used in the DELETE URL.          │
│   This is a compensatable pair."                                            │
│                                                                              │
│                              ▼                                               │
│                                                                              │
│  Suggested Mapping (for human approval):                                     │
│  {                                                                           │
│    "tool": "POST api.airline.com/api/flights/book",                         │
│    "compensator": "DELETE api.airline.com/api/flights/{booking_id}",        │
│    "parameter_mapping": {                                                   │
│      "booking_id": "result.booking_id"                                      │
│    },                                                                        │
│    "confidence": 0.92,                                                      │
│    "suggested_by": "traffic_analysis"                                       │
│  }                                                                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4. Automatic Rollback Execution

When rollback is triggered, the system replays compensation HTTP calls:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Automatic Rollback Execution                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Transaction Log (from eBPF):                                                │
│  ───────────────────────────                                                │
│  1. POST api.airline.com/api/flights/book                                   │
│     Input: {flight_id: "UA-123", passenger: "John"}                         │
│     Output: {booking_id: "BK-789"}                                          │
│                                                                              │
│  2. POST api.hotel.com/reservations                                         │
│     Input: {hotel: "Hilton", guest: "John"}                                 │
│     Output: {reservation_id: "RES-456"}                                     │
│                                                                              │
│  3. POST api.email.com/send  (no compensator - will skip)                   │
│                                                                              │
│                              ▼                                               │
│                      FAILURE DETECTED                                        │
│                              ▼                                               │
│                                                                              │
│  Rollback Plan (LIFO):                                                       │
│  ─────────────────────                                                      │
│  Step 1: SKIP email (no compensation)                                        │
│  Step 2: DELETE api.hotel.com/reservations/RES-456                          │
│  Step 3: DELETE api.airline.com/api/flights/BK-789                          │
│                                                                              │
│                              ▼                                               │
│                                                                              │
│  Execution (automatic HTTP calls):                                           │
│  ─────────────────────────────────                                          │
│  → DELETE api.hotel.com/reservations/RES-456    ✓ 200 OK                   │
│  → DELETE api.airline.com/api/flights/BK-789    ✓ 200 OK                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Implementation Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    eBPF Auto-Instrumentation Stack                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │                        Agent Container                              │    │
│  │  ┌──────────────────────────────────────────────────────────────┐ │    │
│  │  │  Any Agent Code (LangChain, CrewAI, Custom, etc.)            │ │    │
│  │  │                                                               │ │    │
│  │  │  # No AMP SDK needed!                                        │ │    │
│  │  │  def book_flight(flight_id):                                 │ │    │
│  │  │      response = requests.post(                               │ │    │
│  │  │          "https://api.airline.com/book",                     │ │    │
│  │  │          json={"flight_id": flight_id}                       │ │    │
│  │  │      )                                                        │ │    │
│  │  │      return response.json()                                  │ │    │
│  │  │                                                               │ │    │
│  │  └──────────────────────────────────────────────────────────────┘ │    │
│  └──────────────────────────────┬─────────────────────────────────────┘    │
│                                 │                                           │
│                                 │ Network Traffic (HTTP/HTTPS)              │
│                                 ▼                                           │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │                     eBPF Interception Layer                         │    │
│  │                                                                      │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │    │
│  │  │   TC eBPF    │  │  kprobe/     │  │   SSL/TLS Interception   │ │    │
│  │  │   (Traffic   │  │  tracepoint  │  │   (for HTTPS - optional) │ │    │
│  │  │   Control)   │  │  hooks       │  │                          │ │    │
│  │  └──────┬───────┘  └──────┬───────┘  └────────────┬─────────────┘ │    │
│  │         │                 │                        │               │    │
│  │         └─────────────────┴────────────────────────┘               │    │
│  │                           │                                         │    │
│  │                    Ring Buffer / Perf Buffer                        │    │
│  │                           │                                         │    │
│  └───────────────────────────┼─────────────────────────────────────────┘    │
│                              │                                              │
│                              ▼                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │                     eBPF Agent (Go/Rust)                            │    │
│  │                                                                      │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │    │
│  │  │   HTTP       │  │   Tool       │  │   Transaction            │ │    │
│  │  │   Parser     │  │   Classifier │  │   Logger                 │ │    │
│  │  └──────────────┘  └──────────────┘  └──────────────────────────┘ │    │
│  │                                                                      │    │
│  │  ┌──────────────────────────────────────────────────────────────┐ │    │
│  │  │                    LLM Analysis Engine                        │ │    │
│  │  │  - Infer tool schemas from traffic                           │ │    │
│  │  │  - Discover compensation patterns                            │ │    │
│  │  │  - Generate parameter mappings                               │ │    │
│  │  └──────────────────────────────────────────────────────────────┘ │    │
│  │                           │                                         │    │
│  └───────────────────────────┼─────────────────────────────────────────┘    │
│                              │                                              │
│                              ▼                                              │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │                      AMP Backend                                    │    │
│  │                                                                      │    │
│  │  - Stores auto-discovered tool schemas                              │    │
│  │  - Manages compensation mappings (human approval still required)    │    │
│  │  - Executes rollback by replaying compensation HTTP calls          │    │
│  │                                                                      │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Key Components

### 1. Traffic Capture (eBPF)

```c
// TC (Traffic Control) eBPF program for HTTP capture
SEC("tc")
int capture_http(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;

    // Only TCP
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;

    struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;

    // Check for HTTP (port 80/443, or common API ports)
    __u16 dest_port = bpf_ntohs(tcp->dest);
    if (dest_port != 80 && dest_port != 443 && dest_port != 8080)
        return TC_ACT_OK;

    // Extract HTTP payload
    void *http_data = (void *)tcp + (tcp->doff * 4);
    if (http_data + 7 > data_end)
        return TC_ACT_OK;

    // Check for HTTP methods
    char method[8];
    bpf_probe_read_kernel(method, 7, http_data);

    if (__builtin_memcmp(method, "POST ", 5) == 0 ||
        __builtin_memcmp(method, "DELETE", 6) == 0 ||
        __builtin_memcmp(method, "PUT ", 4) == 0) {

        // Capture this request
        struct http_event event = {
            .container_id = get_container_id(),
            .src_port = bpf_ntohs(tcp->source),
            .dst_port = dest_port,
            .dst_ip = ip->daddr,
        };

        // Copy HTTP data (up to limit)
        bpf_probe_read_kernel(event.data, sizeof(event.data), http_data);

        bpf_perf_event_output(skb, &events, BPF_F_CURRENT_CPU,
                              &event, sizeof(event));
    }

    return TC_ACT_OK;
}
```

### 2. HTTP Parser (Userspace Go)

```go
type HTTPTransaction struct {
    ContainerID string
    Method      string
    URL         string
    Headers     map[string]string
    Body        json.RawMessage
    Response    json.RawMessage
    Timestamp   time.Time
}

func parseHTTPEvent(event *ebpf.HTTPEvent) (*HTTPTransaction, error) {
    // Parse raw HTTP from eBPF event
    lines := strings.Split(string(event.Data), "\r\n")

    // First line: "POST /api/flights/book HTTP/1.1"
    parts := strings.SplitN(lines[0], " ", 3)
    method := parts[0]
    path := parts[1]

    // Find Host header
    host := ""
    for _, line := range lines[1:] {
        if strings.HasPrefix(line, "Host:") {
            host = strings.TrimSpace(strings.TrimPrefix(line, "Host:"))
            break
        }
    }

    // Find body (after empty line)
    bodyStart := strings.Index(string(event.Data), "\r\n\r\n")
    body := ""
    if bodyStart > 0 {
        body = string(event.Data[bodyStart+4:])
    }

    return &HTTPTransaction{
        ContainerID: event.ContainerID,
        Method:      method,
        URL:         fmt.Sprintf("%s%s", host, path),
        Body:        json.RawMessage(body),
        Timestamp:   time.Now(),
    }, nil
}
```

### 3. Tool Classifier (LLM-based)

```go
func classifyTool(tx *HTTPTransaction, history []*HTTPTransaction) (*ToolSchema, error) {
    prompt := fmt.Sprintf(`
Analyze this HTTP request and classify it as a tool call.

Request:
- Method: %s
- URL: %s
- Body: %s

Previous requests from this agent:
%s

Respond with JSON:
{
    "tool_name": "descriptive name for this operation",
    "is_side_effect": true/false,
    "input_schema": { extracted from request body },
    "output_schema": { expected from response },
    "potential_compensator": {
        "method": "DELETE/POST/PUT",
        "url_pattern": "URL with {param} placeholders",
        "param_source": "which response field to use"
    }
}
`, tx.Method, tx.URL, tx.Body, formatHistory(history))

    response := callLLM(prompt)

    var schema ToolSchema
    json.Unmarshal(response, &schema)
    return &schema, nil
}
```

### 4. Compensation Executor

```go
func executeCompensation(tx *Transaction, mapping *CompensationMapping) error {
    // Build compensation URL
    url := mapping.CompensatorURL
    for param, source := range mapping.ParameterMapping {
        value := extractValue(source, tx.Input, tx.Output)
        url = strings.Replace(url, "{"+param+"}", value, 1)
    }

    // Build request body if needed
    var body io.Reader
    if mapping.CompensatorBody != nil {
        bodyData := buildBody(mapping.CompensatorBody, tx.Input, tx.Output)
        body = bytes.NewReader(bodyData)
    }

    // Execute compensation request
    req, _ := http.NewRequest(mapping.CompensatorMethod, url, body)
    req.Header.Set("Content-Type", "application/json")

    // Copy original auth headers
    for _, header := range mapping.ForwardHeaders {
        if val := tx.Headers[header]; val != "" {
            req.Header.Set(header, val)
        }
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("compensation failed: %w", err)
    }

    if resp.StatusCode >= 400 {
        return fmt.Errorf("compensation returned %d", resp.StatusCode)
    }

    return nil
}
```

## HTTPS Interception Options

For HTTPS traffic, several approaches:

### Option 1: mTLS Proxy (Recommended)

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Agent     │────▶│   mTLS      │────▶│   External  │
│  Container  │     │   Proxy     │     │   API       │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                    Decrypted traffic
                    visible to AMP
                           │
                           ▼
                    ┌─────────────┐
                    │     AMP     │
                    │   Backend   │
                    └─────────────┘
```

### Option 2: eBPF SSL Hooks

Hook into SSL_read/SSL_write to capture decrypted data:

```c
SEC("uprobe/SSL_write")
int trace_ssl_write(struct pt_regs *ctx) {
    void *ssl = (void *)PT_REGS_PARM1(ctx);
    void *buf = (void *)PT_REGS_PARM2(ctx);
    int len = PT_REGS_PARM3(ctx);

    if (!is_amp_container(bpf_get_current_pid_tgid() >> 32))
        return 0;

    struct ssl_event event = {
        .pid = bpf_get_current_pid_tgid() >> 32,
        .len = len,
    };
    bpf_probe_read_user(event.data, min(len, MAX_DATA_SIZE), buf);

    bpf_perf_event_output(ctx, &ssl_events, BPF_F_CURRENT_CPU,
                          &event, sizeof(event));
    return 0;
}
```

### Option 3: CA Injection

Inject AMP's CA certificate into agent containers for transparent TLS inspection.

## Benefits of eBPF Approach

| Aspect | SDK Approach | eBPF Approach |
|--------|--------------|---------------|
| **Agent Changes** | Required | None |
| **Framework Support** | Manual per framework | All frameworks automatically |
| **Trust Model** | Trust agent reporting | Verify at kernel level |
| **Coverage** | Only wrapped tools | All HTTP traffic |
| **Malicious Agents** | Can lie | Cannot evade |
| **Performance** | Varies | Kernel-optimized |
| **Deployment** | Per-agent | Platform-wide |

## Human Approval Still Required

Even with automatic detection, humans approve compensation mappings:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      AMP UI - Discovered Tools                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Agent: travel-booking-agent                                                 │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Discovered Tool: POST api.airline.com/api/flights/book             │   │
│  │  ──────────────────────────────────────────────────────────────     │   │
│  │  First seen: 2024-01-23 10:30:00                                    │   │
│  │  Call count: 15                                                      │   │
│  │                                                                      │   │
│  │  Input Schema (inferred):                                           │   │
│  │  {                                                                   │   │
│  │    "flight_id": "string",                                           │   │
│  │    "passenger": "string"                                            │   │
│  │  }                                                                   │   │
│  │                                                                      │   │
│  │  Output Schema (inferred):                                          │   │
│  │  {                                                                   │   │
│  │    "booking_id": "string",                                          │   │
│  │    "status": "string"                                               │   │
│  │  }                                                                   │   │
│  │                                                                      │   │
│  │  ┌─────────────────────────────────────────────────────────────┐   │   │
│  │  │  Suggested Compensation (90% confidence)                     │   │   │
│  │  │  ─────────────────────────────────────────                  │   │   │
│  │  │  Compensator: DELETE api.airline.com/api/flights/{id}       │   │   │
│  │  │  Parameter: id ← result.booking_id                          │   │   │
│  │  │                                                              │   │   │
│  │  │  Reasoning: "DELETE request to same endpoint with booking_id│   │   │
│  │  │  from response observed 12 times after failed transactions" │   │   │
│  │  │                                                              │   │   │
│  │  │  [✓ Approve]  [✗ Reject]  [Edit Mapping]                    │   │   │
│  │  └─────────────────────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Next Steps for Implementation

1. **Phase 1: Basic HTTP Capture**
   - TC eBPF program for HTTP traffic
   - Userspace parser in Go
   - Store raw transactions

2. **Phase 2: Tool Classification**
   - LLM-based tool schema inference
   - Pattern detection for compensations
   - UI for discovered tools

3. **Phase 3: HTTPS Support**
   - mTLS proxy integration
   - OR SSL hook eBPF programs

4. **Phase 4: Automatic Rollback**
   - HTTP replay engine
   - Header forwarding (auth, etc.)
   - Response validation

---

This is the proper architecture for production AMP - zero code changes required from agent developers.
