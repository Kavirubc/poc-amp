# Travel Booking Agent

Demo agent with compensatable tools for testing the AMP compensation system.

## Tools

| Tool | Description | Compensator |
|------|-------------|-------------|
| `book_flight` | Books a flight reservation | `cancel_flight` |
| `cancel_flight` | Cancels a flight booking | - |
| `create_reservation` | Creates hotel reservation | `cancel_reservation` |
| `cancel_reservation` | Cancels hotel reservation | - |
| `send_confirmation_email` | Sends email (no undo) | None (skipped) |

## Testing Compensation

1. Deploy agent via AMP UI
2. Open the agent's Compensation tab
3. Approve the suggested mappings
4. Call `POST /demo/create-trip` to create test transactions
5. Trigger rollback via AMP to see compensation in action

## API Endpoints

```bash
# Create a flight booking
curl -X POST http://localhost:9001/tools/book_flight \
  -H "Content-Type: application/json" \
  -d '{"flight_id": "UA-1234", "passenger_name": "John Doe"}'

# Cancel a booking
curl -X POST http://localhost:9001/tools/cancel_flight \
  -H "Content-Type: application/json" \
  -d '{"booking_id": "BK-XXXXXXXX"}'

# Create a full trip (flight + hotel + email)
curl -X POST http://localhost:9001/demo/create-trip

# View all bookings
curl http://localhost:9001/bookings

# View all reservations
curl http://localhost:9001/reservations
```

## Local Development

```bash
pip install -r requirements.txt
python main.py
```
