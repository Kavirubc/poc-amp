"""
Travel Booking Agent with Compensatable Tools

This agent demonstrates the AMP compensation system with:
- book_flight / cancel_flight (compensatable pair)
- create_reservation / cancel_reservation (compensatable pair)
- send_confirmation_email (no compensator - will be skipped during rollback)
"""

import os
import uuid
import logging
from typing import Dict, List, Optional
from datetime import datetime
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import requests
from dotenv import load_dotenv

load_dotenv()

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# In-memory storage for demo
bookings: Dict[str, dict] = {}
reservations: Dict[str, dict] = {}
emails_sent: List[dict] = []

# AMP Configuration
AMP_URL = os.getenv("AMP_URL", "http://backend:8080")
AGENT_ID = os.getenv("AGENT_ID", "")

# Tool schemas for registration with AMP
TOOLS = [
    {
        "name": "book_flight",
        "description": "Books a flight reservation for a passenger",
        "inputSchema": {
            "type": "object",
            "properties": {
                "flight_id": {"type": "string", "description": "Flight identifier"},
                "passenger_name": {"type": "string", "description": "Passenger full name"},
                "seat_class": {"type": "string", "enum": ["economy", "business", "first"]}
            },
            "required": ["flight_id", "passenger_name"]
        }
    },
    {
        "name": "cancel_flight",
        "description": "Cancels an existing flight booking",
        "inputSchema": {
            "type": "object",
            "properties": {
                "booking_id": {"type": "string", "description": "Booking ID to cancel"}
            },
            "required": ["booking_id"]
        }
    },
    {
        "name": "create_reservation",
        "description": "Creates a hotel reservation",
        "inputSchema": {
            "type": "object",
            "properties": {
                "hotel_id": {"type": "string", "description": "Hotel identifier"},
                "guest_name": {"type": "string", "description": "Guest full name"},
                "check_in": {"type": "string", "description": "Check-in date (YYYY-MM-DD)"},
                "check_out": {"type": "string", "description": "Check-out date (YYYY-MM-DD)"},
                "room_type": {"type": "string", "enum": ["standard", "deluxe", "suite"]}
            },
            "required": ["hotel_id", "guest_name", "check_in", "check_out"]
        }
    },
    {
        "name": "cancel_reservation",
        "description": "Cancels an existing hotel reservation",
        "inputSchema": {
            "type": "object",
            "properties": {
                "reservation_id": {"type": "string", "description": "Reservation ID to cancel"}
            },
            "required": ["reservation_id"]
        }
    },
    {
        "name": "send_confirmation_email",
        "description": "Sends a confirmation email to the customer (cannot be undone)",
        "inputSchema": {
            "type": "object",
            "properties": {
                "to_email": {"type": "string", "description": "Recipient email address"},
                "subject": {"type": "string", "description": "Email subject"},
                "booking_details": {"type": "object", "description": "Booking details to include"}
            },
            "required": ["to_email", "subject"]
        }
    }
]


def register_tools_with_amp():
    """Register tools with AMP platform for compensation analysis."""
    if not AGENT_ID:
        logger.warning("AGENT_ID not set, skipping AMP registration")
        return

    try:
        response = requests.post(
            f"{AMP_URL}/api/v1/agents/{AGENT_ID}/tools",
            json={"tools": TOOLS},
            timeout=30
        )
        response.raise_for_status()
        result = response.json()
        logger.info(f"Registered {len(TOOLS)} tools with AMP")
        logger.info(f"Suggested mappings: {result.get('suggested_mappings', [])}")
        return result
    except Exception as e:
        logger.error(f"Failed to register tools with AMP: {e}")
        return None


def log_transaction(tool_name: str, input_params: dict, output_result: dict):
    """Log a tool execution to AMP for transaction tracking."""
    if not AGENT_ID:
        return

    try:
        requests.post(
            f"{AMP_URL}/api/v1/agents/{AGENT_ID}/transactions",
            json={
                "session_id": os.getenv("SESSION_ID", str(uuid.uuid4())),
                "tool_name": tool_name,
                "input": input_params,
                "output": output_result
            },
            timeout=10
        )
        logger.debug(f"Logged transaction: {tool_name}")
    except Exception as e:
        logger.warning(f"Failed to log transaction: {e}")


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Register tools on startup."""
    register_tools_with_amp()
    yield


app = FastAPI(
    title="Travel Booking Agent",
    description="Demo agent with compensatable tools for AMP testing",
    version="1.0.0",
    lifespan=lifespan
)


# Request/Response Models
class FlightBookingRequest(BaseModel):
    flight_id: str
    passenger_name: str
    seat_class: str = "economy"


class FlightBookingResponse(BaseModel):
    booking_id: str
    flight_id: str
    passenger_name: str
    seat_class: str
    status: str
    booked_at: str


class CancelRequest(BaseModel):
    booking_id: Optional[str] = None
    reservation_id: Optional[str] = None


class ReservationRequest(BaseModel):
    hotel_id: str
    guest_name: str
    check_in: str
    check_out: str
    room_type: str = "standard"


class ReservationResponse(BaseModel):
    reservation_id: str
    hotel_id: str
    guest_name: str
    check_in: str
    check_out: str
    room_type: str
    status: str
    created_at: str


class EmailRequest(BaseModel):
    to_email: str
    subject: str
    booking_details: Optional[dict] = None


class EmailResponse(BaseModel):
    message_id: str
    to_email: str
    subject: str
    status: str
    sent_at: str


# Tool Endpoints
@app.post("/tools/book_flight", response_model=FlightBookingResponse)
async def book_flight(request: FlightBookingRequest):
    """Book a flight - creates a compensatable transaction."""
    booking_id = f"BK-{uuid.uuid4().hex[:8].upper()}"

    booking = {
        "booking_id": booking_id,
        "flight_id": request.flight_id,
        "passenger_name": request.passenger_name,
        "seat_class": request.seat_class,
        "status": "confirmed",
        "booked_at": datetime.utcnow().isoformat()
    }

    bookings[booking_id] = booking
    logger.info(f"Flight booked: {booking_id}")

    # Log to AMP
    log_transaction(
        "book_flight",
        request.model_dump(),
        booking
    )

    return FlightBookingResponse(**booking)


@app.post("/tools/cancel_flight")
async def cancel_flight(request: CancelRequest):
    """Cancel a flight booking - compensates book_flight."""
    booking_id = request.booking_id

    if not booking_id or booking_id not in bookings:
        raise HTTPException(status_code=404, detail=f"Booking {booking_id} not found")

    booking = bookings[booking_id]
    booking["status"] = "cancelled"
    booking["cancelled_at"] = datetime.utcnow().isoformat()

    logger.info(f"Flight cancelled: {booking_id}")

    # Log to AMP
    log_transaction(
        "cancel_flight",
        {"booking_id": booking_id},
        {"status": "cancelled", "booking_id": booking_id}
    )

    return {"status": "cancelled", "booking_id": booking_id}


@app.post("/tools/create_reservation", response_model=ReservationResponse)
async def create_reservation(request: ReservationRequest):
    """Create a hotel reservation - creates a compensatable transaction."""
    reservation_id = f"RES-{uuid.uuid4().hex[:8].upper()}"

    reservation = {
        "reservation_id": reservation_id,
        "hotel_id": request.hotel_id,
        "guest_name": request.guest_name,
        "check_in": request.check_in,
        "check_out": request.check_out,
        "room_type": request.room_type,
        "status": "confirmed",
        "created_at": datetime.utcnow().isoformat()
    }

    reservations[reservation_id] = reservation
    logger.info(f"Reservation created: {reservation_id}")

    # Log to AMP
    log_transaction(
        "create_reservation",
        request.model_dump(),
        reservation
    )

    return ReservationResponse(**reservation)


@app.post("/tools/cancel_reservation")
async def cancel_reservation(request: CancelRequest):
    """Cancel a hotel reservation - compensates create_reservation."""
    reservation_id = request.reservation_id

    if not reservation_id or reservation_id not in reservations:
        raise HTTPException(status_code=404, detail=f"Reservation {reservation_id} not found")

    reservation = reservations[reservation_id]
    reservation["status"] = "cancelled"
    reservation["cancelled_at"] = datetime.utcnow().isoformat()

    logger.info(f"Reservation cancelled: {reservation_id}")

    # Log to AMP
    log_transaction(
        "cancel_reservation",
        {"reservation_id": reservation_id},
        {"status": "cancelled", "reservation_id": reservation_id}
    )

    return {"status": "cancelled", "reservation_id": reservation_id}


@app.post("/tools/send_confirmation_email", response_model=EmailResponse)
async def send_confirmation_email(request: EmailRequest):
    """Send confirmation email - NOT compensatable (cannot unsend)."""
    message_id = f"MSG-{uuid.uuid4().hex[:8].upper()}"

    email = {
        "message_id": message_id,
        "to_email": request.to_email,
        "subject": request.subject,
        "booking_details": request.booking_details,
        "status": "sent",
        "sent_at": datetime.utcnow().isoformat()
    }

    emails_sent.append(email)
    logger.info(f"Email sent: {message_id} to {request.to_email}")

    # Log to AMP
    log_transaction(
        "send_confirmation_email",
        request.model_dump(),
        {"message_id": message_id, "status": "sent"}
    )

    return EmailResponse(**email)


# Query Endpoints
@app.get("/bookings")
async def list_bookings():
    """List all flight bookings."""
    return {"bookings": list(bookings.values())}


@app.get("/bookings/{booking_id}")
async def get_booking(booking_id: str):
    """Get a specific booking."""
    if booking_id not in bookings:
        raise HTTPException(status_code=404, detail="Booking not found")
    return bookings[booking_id]


@app.get("/reservations")
async def list_reservations():
    """List all hotel reservations."""
    return {"reservations": list(reservations.values())}


@app.get("/reservations/{reservation_id}")
async def get_reservation(reservation_id: str):
    """Get a specific reservation."""
    if reservation_id not in reservations:
        raise HTTPException(status_code=404, detail="Reservation not found")
    return reservations[reservation_id]


@app.get("/emails")
async def list_emails():
    """List all sent emails."""
    return {"emails": emails_sent}


# Demo/Test Endpoints
@app.post("/demo/create-trip")
async def demo_create_trip():
    """
    Demo endpoint that creates a full trip (flight + hotel + email).
    Useful for testing the compensation workflow.
    """
    # Book flight
    flight = await book_flight(FlightBookingRequest(
        flight_id="UA-1234",
        passenger_name="John Doe",
        seat_class="business"
    ))

    # Create hotel reservation
    hotel = await create_reservation(ReservationRequest(
        hotel_id="HILTON-NYC",
        guest_name="John Doe",
        check_in="2024-03-15",
        check_out="2024-03-18",
        room_type="deluxe"
    ))

    # Send confirmation
    email = await send_confirmation_email(EmailRequest(
        to_email="john.doe@example.com",
        subject="Your Trip Confirmation",
        booking_details={
            "flight": flight.model_dump(),
            "hotel": hotel.model_dump()
        }
    ))

    return {
        "message": "Trip created successfully",
        "flight": flight,
        "hotel": hotel,
        "email": email
    }


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "agent": "travel-booking"}


@app.get("/")
async def root():
    """Root endpoint with API info."""
    return {
        "name": "Travel Booking Agent",
        "version": "1.0.0",
        "description": "Demo agent with compensatable tools for AMP testing",
        "tools": [t["name"] for t in TOOLS],
        "endpoints": {
            "book_flight": "POST /tools/book_flight",
            "cancel_flight": "POST /tools/cancel_flight",
            "create_reservation": "POST /tools/create_reservation",
            "cancel_reservation": "POST /tools/cancel_reservation",
            "send_confirmation_email": "POST /tools/send_confirmation_email",
            "demo_create_trip": "POST /demo/create-trip"
        }
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
