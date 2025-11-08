package main

import (
	"encoding/base64"
	"encoding/binary"
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

// handleDrawingStream processes a bidirectional stream for drawing operations ONLY.
// This is completely separate from file operations.
func handleDrawingStream(server *MessageServer, client *Client, s *webtransport.Stream) {
	defer func() {
		s.Close()
		log.Printf("[%s] Drawing stream closed", client.Name)
	}()

	// Read header with fixed-length prefix
	hdr, err := readDrawingHeaderFixed(s)
	if err != nil {
		log.Printf("[%s] Error reading drawing header: %v", client.Name, err)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": err.Error()})
		return
	}

	// Validate operation type
	if hdr.Op != "drawing" {
		log.Printf("[%s] Invalid drawing operation: %s", client.Name, hdr.Op)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid operation: expected 'drawing'"})
		return
	}

	// Validate size
	if hdr.Size <= 0 || hdr.Size > 10*1024*1024 { // Max 10MB
		log.Printf("[%s] Invalid drawing size: %d", client.Name, hdr.Size)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid drawing size"})
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
			writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "failed to read image data"})
			return
		}
		totalRead += int64(n)
	}

	if totalRead != hdr.Size {
		log.Printf("[%s] Size mismatch: expected %d, got %d", client.Name, hdr.Size, totalRead)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "size mismatch"})
		return
	}

	log.Printf("[%s] Drawing received successfully (%.2f KB)", 
		client.Name, float64(totalRead)/1024)

	// Encode to base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	// Send success response immediately
	responseData := map[string]interface{}{
		"status": "ok",
		"size":   totalRead,
	}
	respBytes, _ := json.Marshal(responseData)
	respBytes = append(respBytes, '\n')
	
	if _, err := s.Write(respBytes); err != nil {
		log.Printf("[%s] Failed to send drawing response: %v", client.Name, err)
		return
	}

	log.Printf("[%s] Drawing response sent to client", client.Name)

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

// readDrawingHeaderFixed reads header with fixed-length prefix (4 bytes for header length)
func readDrawingHeaderFixed(s io.Reader) (*drawingHeader, error) {
	// Read 4 bytes for header length
	headerLengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(s, headerLengthBuf); err != nil {
		return nil, fmt.Errorf("failed to read header length: %w", err)
	}
	
	headerLength := binary.BigEndian.Uint32(headerLengthBuf)
	
	// Sanity check
	if headerLength == 0 || headerLength > 16*1024 { // Max 16KB for header
		return nil, fmt.Errorf("invalid header length: %d bytes", headerLength)
	}
	
	// Read exact header JSON
	headerBuf := make([]byte, headerLength)
	if _, err := io.ReadFull(s, headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read header JSON: %w", err)
	}
	
	var hdr drawingHeader
	if err := json.Unmarshal(headerBuf, &hdr); err != nil {
		return nil, fmt.Errorf("invalid drawing header format: %w", err)
	}
	
	return &hdr, nil
}

// writeDrawingJSONResult is specific for drawing responses
func writeDrawingJSONResult(w io.Writer, v interface{}) {
	b, _ := json.Marshal(v)
	b = append(b, '\n')
	w.Write(b)
}