package bridge

import (
	"fmt"
	"io"
	"net/http"
)

const mjpegBoundary = "obsframe"

func (b *Bridge) httpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream/{id}", b.handleStream)
	mux.HandleFunc("GET /snapshot/{id}", b.handleSnapshot)
	return mux
}

func (b *Bridge) handleStream(w http.ResponseWriter, r *http.Request) {
	conn, ok := b.conns[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	frames := conn.subscribe()
	defer conn.unsubscribe(frames)

	header := w.Header()
	header.Set("Content-Type", "multipart/x-mixed-replace; boundary="+mjpegBoundary)
	header.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", mjpegBoundary, len(frame)); err != nil {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\r\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (b *Bridge) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	conn, ok := b.conns[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	frame := conn.latestFrame()
	if frame == nil {
		http.Error(w, "no frame captured yet", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(frame)
}
