# LangChain Gemini Agent

A simple LangChain agent using Google's Gemini model with tools for calculation, time, and word counting.

## Setup

1. Set your Google API key:
```bash
export GOOGLE_API_KEY=your-key-here
```

2. Install dependencies:
```bash
pip install -r requirements.txt
```

3. Run the server:
```bash
python main.py
```

## API Endpoints

- `GET /` - Health check
- `GET /health` - Health status
- `POST /chat` - Send a message to the agent

### Example

```bash
curl -X POST http://localhost:8000/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "What is 25 * 4 + 10?"}'
```
