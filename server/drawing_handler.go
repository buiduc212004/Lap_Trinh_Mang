package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/quic-go/webtransport-go"
)

// drawingHeader defines the structure of the JSON header for drawing operations.
type drawingHeader struct {
	Op     string `json:"op"`
	Size   int64  `json:"size,omitempty"`
	Format string `json:"format,omitempty"`
}

// handleDrawingStream processes a bidirectional stream for drawing operations.
func handleDrawingStream(server *MessageServer, client *Client, s *webtransport.Stream) {
	defer s.Close()

	// Read header
	hdr, err := readDrawingHeader(s)
	if err != nil {
		log.Printf("[%s] Error reading drawing header: %v", client.Name, err)
		writeJSONResult(s, map[string]string{"status": "error", "error": err.Error()})
		return
	}

	if hdr.Op != "drawing" {
		writeJSONResult(s, map[string]string{"status": "error", "error": "invalid operation"})
		return
	}

	log.Printf("[%s] Receiving drawing (%s, %.2f KB)", 
		client.Name, hdr.Format, float64(hdr.Size)/1024)

	// Read image data
	imageData := make([]byte, hdr.Size)
	totalRead := int64(0)
	
	for totalRead < hdr.Size {
		n, err := s.Read(imageData[totalRead:])
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("[%s] Error reading drawing data: %v", client.Name, err)
			writeJSONResult(s, map[string]string{"status": "error", "error": "failed to read image data"})
			return
		}
		totalRead += int64(n)
	}

	if totalRead != hdr.Size {
		log.Printf("[%s] Size mismatch: expected %d, got %d", client.Name, hdr.Size, totalRead)
		writeJSONResult(s, map[string]string{"status": "error", "error": "size mismatch"})
		return
	}

	log.Printf("[%s] Drawing received successfully (%.2f KB)", 
		client.Name, float64(totalRead)/1024)

	// Encode to base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	// Send success response
	writeJSONResult(s, map[string]interface{}{
		"status": "ok",
		"size":   totalRead,
	})

	// Broadcast drawing to all clients
	go func() {
		msg, _ := json.Marshal(map[string]interface{}{
			"type": "drawing",
			"name": client.Name,
			"data": base64Data,
		})
		server.Broadcast(msg)
		log.Printf("[%s] Drawing broadcasted to all clients", client.Name)
	}()
}

// readDrawingHeader reads the initial JSON line from the drawing stream.
func readDrawingHeader(s io.Reader) (*drawingHeader, error) {
	headerBuf := make([]byte, 0, 4*1024)
	tmp := make([]byte, 1024)
	
	for {
		n, err := s.Read(tmp)
		if n > 0 {
			headerBuf = append(headerBuf, tmp[:n]...)
			if i := bytes.IndexByte(headerBuf, '\n'); i >= 0 {
				headerBuf = headerBuf[:i]
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("reading drawing header failed: %w", err)
		}
		if len(headerBuf) > 4*1024 {
			return nil, fmt.Errorf("drawing header too large")
		}
	}

	var hdr drawingHeader
	if err := json.Unmarshal(headerBuf, &hdr); err != nil {
		return nil, fmt.Errorf("invalid drawing header format: %w", err)
	}
	
	return &hdr, nil
}