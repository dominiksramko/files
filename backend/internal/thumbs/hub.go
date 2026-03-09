package thumbs

import "sync"

// ThumbEvent carries the content hash of a newly-ready thumbnail.
type ThumbEvent struct {
	Hash string
}

// hub broadcasts thumbnail-ready events to SSE subscribers, keyed by folder abs path.
type hub struct {
	mu   sync.Mutex
	subs map[string][]chan ThumbEvent
}

func newHub() *hub {
	return &hub{subs: make(map[string][]chan ThumbEvent)}
}

// Subscribe returns a channel that receives ThumbEvents when thumbnails are
// ready, plus an unsubscribe function the caller must defer.
func (h *hub) Subscribe(folderAbs string) (<-chan ThumbEvent, func()) {
	ch := make(chan ThumbEvent, 64)
	h.mu.Lock()
	h.subs[folderAbs] = append(h.subs[folderAbs], ch)
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subs[folderAbs]
		for i, c := range subs {
			if c == ch {
				h.subs[folderAbs] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(h.subs[folderAbs]) == 0 {
			delete(h.subs, folderAbs)
		}
		close(ch)
	}
}

// Broadcast sends a ThumbEvent to all subscribers watching folderAbs.
// Non-blocking: slow subscribers are skipped (channel is buffered).
func (h *hub) Broadcast(folderAbs, hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs[folderAbs] {
		select {
		case ch <- ThumbEvent{Hash: hash}:
		default:
		}
	}
}
