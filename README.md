# Server Monitoring — Endpoints Documentation

This document lists all HTTP and WebSocket endpoints provided by the server-monitoring service, the expected payloads, example requests, and the Elasticsearch indices used.

## Endpoints

- `GET /` — Service info and list of endpoints.
- `GET /health` — Health check endpoint. Returns service status and number of connected monitor clients.
- `POST /track` — HTTP endpoint to send a `CodingSession` JSON payload to be indexed into Elasticsearch.
- `ws://<host>/ws/track` — WebSocket endpoint where clients can send `CodingSession` messages in real time.
- `ws://<host>/ws/monitor` — WebSocket endpoint for clients that want to receive broadcast messages (system metrics and session events).

---

## CodingSession payload (example)

This is the JSON structure accepted by both `POST /track` and `ws://.../ws/track`:

```json
{
  "duration_seconds": 120,
  "editor": "vscode",
  "project": "my-project",
  "language": "go",
  "file_path": "/path/to/file.go",
  "timestamp": "2025-11-29T12:34:56Z",
  "lines_of_code": 42
}
```

Notes:

- `file_path` and `lines_of_code` are optional.

- The server will add a `server_timestamp` when indexing into Elasticsearch.

Elasticsearch index used: `coding-sessions`.

---

## WebSocket `/ws/track` (client -> server)

- Purpose: Clients (agents, editor plugins, or other services) send `CodingSession` JSON payloads.
- Server behavior: index the session into Elasticsearch and reply with an acknowledgement JSON:

```json
{ "status": "received", "timestamp": "2025-11-29T12:34:57Z" }
```

- The session is also broadcast to all clients connected to `/ws/monitor`.

---

## WebSocket `/ws/monitor` (server -> clients)

- Purpose: Clients connect to receive real-time `metrics` and `session` events.
- Broadcast format (wrapper `BroadcastMessage`):

```json
{
  "type": "metrics", // or "session"
  "data": { /* SystemMetrics or CodingSession object */ },
  "event_id": "20251129123457"
}
```

Example `SystemMetrics` (fields populated by gopsutil):

```json
{
  "cpu": 12.5,
  "cpu_model": "Intel(R) Core(TM) i7-...",
  "cores": 8,
  "memory": 32.1,
  "total_mem": 16,
  "used_mem": 3,
  "os": "linux",
  "platform": "ubuntu",
  "kernel": "5.15.0-...",
  "arch": "x86_64",
  "uptime": 123456,
  "timestamp": "2025-11-29T12:34:56Z"
}
```

---

## HTTP `POST /track` (alternative to WebSocket)

- Purpose: For clients that cannot use WebSocket. Send a `CodingSession` JSON payload with `Content-Type: application/json`.
- Successful response example:

```json
{ "status": "received", "timestamp": "2025-11-29T12:34:57Z" }
```

Status codes:

- `200 OK` — payload received and indexed.

- `400 Bad Request` — invalid JSON.

- `500 Internal Server Error` — failed to index into Elasticsearch.

---

## `GET /health`

- Returns a short JSON payload, e.g.:

```json
{
  "status": "ok",
  "elasticsearch": "connected",
  "connected_clients": 3
}
```

---

## Examples (curl and websocat)

- POST via curl:

```bash
curl -X POST http://localhost:8080/track \
  -H "Content-Type: application/json" \
  -d '{"duration_seconds":60,"editor":"vscode","project":"repo","language":"go","timestamp":"2025-11-29T12:00:00Z"}'
```

- WebSocket send (using websocat):

```bash
# send a session via websocket
printf '{"duration_seconds":60,"editor":"vscode","project":"repo","language":"go","timestamp":"2025-11-29T12:00:00Z"}' | websocat ws://localhost:8080/ws/track

# listen for monitor broadcasts
websocat ws://localhost:8080/ws/monitor
```

---

## Elasticsearch indices

- `coding-sessions` — stored CodingSession documents.

- `system-metrics` — periodic system metrics.

---

## Notes

- Ensure Elasticsearch is running at `http://localhost:9200` or update the address in `NewESClient()`.

- If you want to change the index names, update `IndexSession` / `IndexMetrics` functions.

---

If you want, I can:

- Add more concrete examples for all fields.

- Move struct definitions into `types.go` and update README to reference those types.

- Add tiny integration tests for `POST /track`.
