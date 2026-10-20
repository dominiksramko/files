package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

// startEventStream sends the headers of a server-sent event stream right away,
// so the client knows it's open before the first event.
func startEventStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable proxy buffering
	c.Writer.Flush()
}

// writeEvent sends one event with payload as JSON; an empty event name sends
// an unnamed one (EventSource's onmessage). A failed write means the client is
// gone.
func writeEvent(c *gin.Context, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if event != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
