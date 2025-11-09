package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
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

// handleDrawingStreamWithPeek handles drawing with already-read peek bytes
func handleDrawingStreamWithPeek(server *MessageServer, client *Client, s *webtransport.Stream, peekData []byte) {
	log.Printf("[%s] Drawing stream started", client.Name)

	// Bọc stream bằng bufio.Reader để đọc liên tục
	br := bufio.NewReader(s)

	// Đọc header với length-prefix
	hdr, remainingData, err := readDrawingHeaderWithPeek(br, peekData)
	if err != nil {
		log.Printf("[%s] Error reading drawing header: %v", client.Name, err)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": err.Error()})
		return
	}

	// Kiểm tra loại operation
	if hdr.Op != "drawing" {
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid operation"})
		return
	}

	// Kiểm tra size hợp lệ
	if hdr.Size <= 0 || hdr.Size > 10*1024*1024 {
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid drawing size"})
		return
	}

	log.Printf("[%s] Receiving drawing (%s, %.2f KB)", client.Name, hdr.Format, float64(hdr.Size)/1024)

	// Buffer ảnh
	imageData := make([]byte, hdr.Size)
	totalRead := int64(copy(imageData, remainingData))
	if totalRead > 0 {
		log.Printf("[%s] Copied %d remaining bytes from header read", client.Name, totalRead)
	}

	// Đọc tiếp cho đến khi đủ hdr.Size byte
	for totalRead < hdr.Size {
		n, err := br.Read(imageData[totalRead:])
		if err != nil {
			if err == io.EOF {
				log.Printf("[%s] Unexpected EOF at %d/%d bytes", client.Name, totalRead, hdr.Size)
				writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "unexpected EOF"})
				return
			}
			log.Printf("[%s] Error reading drawing data: %v", client.Name, err)
			writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "failed to read image data"})
			return
		}
		totalRead += int64(n)
		log.Printf("[%s] Read %d bytes (total %d/%d)", client.Name, n, totalRead, hdr.Size)
	}

	// Kiểm tra cuối cùng
	if totalRead != hdr.Size {
		log.Printf("[%s] Size mismatch: expected %d, got %d", client.Name, hdr.Size, totalRead)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "size mismatch"})
		return
	}

	log.Printf("[%s] Drawing received successfully (%.2f KB)", client.Name, float64(totalRead)/1024)

	// Encode sang base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)

	// Gửi phản hồi thành công
	responseData := map[string]interface{}{"status": "ok", "size": totalRead}
	respBytes, _ := json.Marshal(responseData)
	respBytes = append(respBytes, '\n')
	if _, err := s.Write(respBytes); err != nil {
		log.Printf("[%s] Failed to send drawing response: %v", client.Name, err)
		return
	}
	log.Printf("[%s] Drawing response sent to client", client.Name)

	// Broadcast tới tất cả client khác
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

// writeDrawingJSONResult is specific for drawing responses
func writeDrawingJSONResult(w io.Writer, v interface{}) {
	b, _ := json.Marshal(v)
	b = append(b, '\n')
	w.Write(b)
}
