# AMP Instrumentation

Python library for instrumenting AI agents with the AMP (Agent Management Platform) for automatic compensation and rollback support.

## Installation

```bash
pip install amp-instrumentation
```

Or install from source:

```bash
cd amp-instrumentation
pip install -e .
```

## Quick Start

```python
from amp_instrumentation import AMPInterceptor

# Initialize the interceptor
interceptor = AMPInterceptor(
    agent_id="your-agent-uuid",
    amp_url="http://localhost:8080"
)

# Register your tools for compensation analysis
interceptor.register_tools([
    {
        "name": "book_flight",
        "description": "Books a flight reservation",
        "inputSchema": {
            "type": "object",
            "properties": {
                "flight_id": {"type": "string"},
                "passenger_id": {"type": "string"}
            }
        }
    },
    {
        "name": "cancel_flight",
        "description": "Cancels a flight reservation",
        "inputSchema": {
            "type": "object",
            "properties": {
                "booking_id": {"type": "string"}
            }
        }
    }
])

# Wrap your tool functions
@interceptor.wrap_tool("book_flight")
def book_flight(flight_id: str, passenger_id: str) -> dict:
    # Your implementation
    return {"booking_id": "BK123", "status": "confirmed"}

@interceptor.wrap_tool("cancel_flight")
def cancel_flight(booking_id: str) -> dict:
    # Your implementation
    return {"status": "cancelled"}

# Use tools normally - they're automatically logged
result = book_flight("FL001", "PAX123")

# On failure, get rollback plan
plan = interceptor.get_rollback_plan()

# Or execute rollback automatically
rollback_result = interceptor.execute_rollback()
```

## Features

### Tool Registration

Register tools with AMP to get automatic compensation suggestions:

```python
interceptor.register_tools([
    {
        "name": "create_order",
        "description": "Creates a new order",
        "inputSchema": {...}
    }
])
```

AMP will analyze the tools and suggest:
- Which tools have side effects
- What the compensating tool should be (e.g., `cancel_order`)
- How to map parameters (e.g., `order_id` from result)

### Transaction Logging

Every tool call is automatically logged:

```python
@interceptor.wrap_tool("send_email")
def send_email(to: str, subject: str, body: str):
    # Implementation
    pass

# This call is automatically logged
send_email("user@example.com", "Hello", "World")
```

### Rollback Support

When failures occur, you can rollback:

```python
# Get the plan first
plan = interceptor.get_rollback_plan()
print(f"Will compensate {len(plan['steps'])} operations")

# Execute rollback
result = interceptor.execute_rollback(
    tool_executor=lambda name, params: tools[name](**params)
)
print(f"Compensated: {result['compensated']}, Skipped: {result['skipped']}")
```

## Compensation Registry

For more control, use the CompensationRegistry directly:

```python
from amp_instrumentation import CompensationRegistry

registry = CompensationRegistry(
    agent_id="your-agent-uuid",
    amp_url="http://localhost:8080"
)

# Sync with AMP
registry.sync()

# Check if tool has compensator
if registry.has_compensator("book_flight"):
    mapping = registry.get_compensator("book_flight")
    print(f"Compensator: {mapping['compensator']}")

# Register local compensator functions
registry.register_compensator("book_flight", cancel_flight)

# Execute compensation
registry.execute_compensation(
    "book_flight",
    original_input={"flight_id": "FL001"},
    original_output={"booking_id": "BK123"}
)
```

## Recovery Manager

For complex rollback scenarios:

```python
from amp_instrumentation import CompensationRegistry, RecoveryManager

registry = CompensationRegistry(agent_id, amp_url)
recovery = RecoveryManager(registry)

# Record transactions manually
recovery.record_transaction(
    transaction_id="tx-1",
    tool_name="book_flight",
    input_params={"flight_id": "FL001"},
    output_result={"booking_id": "BK123"}
)

# Generate and review plan
plan = recovery.generate_rollback_plan()
for step in plan:
    print(f"{step.tool_name}: {step.action}")

# Execute
result = recovery.execute_rollback()
```

## Human Approval Workflow

1. Register tools with AMP
2. AMP suggests compensation mappings
3. Human reviews in AMP UI (http://localhost:3000)
4. Approve/reject mappings
5. Only approved mappings are used for automatic compensation

This ensures humans remain in control of what compensations are allowed.
