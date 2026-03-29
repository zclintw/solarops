package hub

import "sync"

type Hub struct {
    clients map[chan []byte]struct{}
    mu      sync.RWMutex
}

func New() *Hub {
    return &Hub{
        clients: make(map[chan []byte]struct{}),
    }
}

func (h *Hub) Register(ch chan []byte) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.clients[ch] = struct{}{}
}

func (h *Hub) Unregister(ch chan []byte) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if _, ok := h.clients[ch]; ok {
        delete(h.clients, ch)
        close(ch)
    }
}

func (h *Hub) Broadcast(data []byte) {
    h.mu.RLock()
    defer h.mu.RUnlock()

    for ch := range h.clients {
        select {
        case ch <- data:
        default:
            // Client too slow, skip
        }
    }
}

func (h *Hub) ClientCount() int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    return len(h.clients)
}
