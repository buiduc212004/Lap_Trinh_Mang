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

// handleDrawingStreamWithPeek xử lý bản vẽ với dữ liệu đã đọc (peek)
func handleDrawingStreamWithPeek(server *MessageServer, client *Client, s *webtransport.Stream, peekData []byte) {
	log.Printf("[%s] Drawing stream started", client.Name)

	// SỬA LỖI: Khâu peekData trở lại đầu stream
	// Tạo một reader "ảo" cho peekData
	peekReader := &bytesReader{data: peekData}
	// Kết hợp peekReader và stream chính
	fullStreamReader := io.MultiReader(peekReader, s)

	// Bọc reader đã khâu bằng bufio.Reader
	// Bây giờ br đọc từ peekData trước, sau đó mới đến 's'
	br := bufio.NewReader(fullStreamReader)

	// Đọc header với length-prefix TỪ BUFIO.READER
	// Chúng ta KHÔNG cần truyền peekData vào đây nữa vì nó đã được khâu vào br
	// Chúng ta cần một hàm readDrawingHeader mới không lấy peekData

	// --- Bắt đầu thay đổi logic đọc header ---

	// 1. Đọc 4 byte độ dài header (Big Endian)
	headerLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(br, headerLenBytes); err != nil {
		log.Printf("[%s] Error reading drawing header length: %v", client.Name, err)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "failed to read header length"})
		return
	}
	headerLength := uint32(headerLenBytes[0])<<24 | uint32(headerLenBytes[1])<<16 | uint32(headerLenBytes[2])<<8 | uint32(headerLenBytes[3])

	if headerLength == 0 || headerLength > 16*1024 { // Giới hạn 16KB
		log.Printf("[%s] Invalid header length: %d bytes", client.Name, headerLength)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid header length"})
		return
	}

	// 2. Đọc chính xác header JSON
	headerJSON := make([]byte, headerLength)
	if _, err := io.ReadFull(br, headerJSON); err != nil {
		log.Printf("[%s] Error reading drawing header JSON: %v", client.Name, err)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "failed to read header JSON"})
		return
	}

	// 3. Phân tích JSON
	var hdr drawingHeader
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		log.Printf("[%s] Invalid drawing header format: %v", client.Name, err)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid drawing header format"})
		return
	}
	// --- Kết thúc thay đổi logic đọc header ---

	// Kiểm tra loại operation
	if hdr.Op != "drawing" {
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid operation"})
		return
	}

	// Kiểm tra size hợp lệ
	if hdr.Size <= 0 || hdr.Size > 10*1024*1024 { // 10MB limit
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "invalid drawing size"})
		return
	}

	log.Printf("[%s] Receiving drawing (%s, %.2f KB)", client.Name, hdr.Format, float64(hdr.Size)/1024)

	// Buffer ảnh
	imageData := make([]byte, hdr.Size)

	// SỬA LỖI: Không còn 'remainingData'. Đọc trực tiếp từ 'br'
	totalRead, err := io.ReadFull(br, imageData)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			log.Printf("[%s] Unexpected EOF at %d/%d bytes", client.Name, totalRead, hdr.Size)
			writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "unexpected EOF"})
			return
		}
		log.Printf("[%s] Error reading drawing data: %v", client.Name, err)
		writeDrawingJSONResult(s, map[string]string{"status": "error", "error": "failed to read image data"})
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
