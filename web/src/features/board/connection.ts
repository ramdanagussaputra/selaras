// The current board WebSocket connection id, held in memory (like the access
// token in authClient). useBoardChannel sets it on `welcome`; the REST mutations
// read it to stamp the X-Conn-Id header so this connection suppresses its own
// echo (design D7). Only one board view is open at a time, so a single value is
// enough.

let connectionId: string | null = null;

export function setConnectionId(id: string | null): void {
  connectionId = id;
}

export function getConnectionId(): string | null {
  return connectionId;
}
