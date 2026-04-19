package api

// WebSocket support is reserved for a future iteration.
// The SSE-based streaming in http.go covers the v0.1 use case.
// A WebSocket handler would follow the same pattern:
//   1. Upgrade the connection.
//   2. Read a JSON message from the client.
//   3. Call engine.ProcessTurn.
//   4. Forward textCh chunks as WebSocket text frames.
//   5. Send a final "done" frame with the TurnResult JSON.
