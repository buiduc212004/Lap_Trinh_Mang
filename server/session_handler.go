package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/quic-go/webtransport-go"
)

// handleWebTransportSession manages a new client connection.
func handleWebTransportSession(messageServer *MessageServer, sessionID int, session *webtransport.Session, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Anonymous"
	}
	log.Printf("Session #%d started. Client: %s", sessionID, name)

	// Open a persistent unidirectional stream for server->client messages
	sendStream, err := session.OpenUniStream()
	if err != nil {
		log.Printf("[%s] Failed to open persistent UniStream: %s", name, err)
		return
	}

	client := &Client{
		Name:       name,
		Session:    session,
		Ch:         make(chan []byte, 10),
		SendStream: sendStream,
	}

	messageServer.AddClient(client)
	messageServer.BroadcastOnlineList()
	messageServer.SendFileList(client)

	// Announce join
	joinMsg, _ := json.Marshal(map[string]string{"type": "system", "message": name + " joined the chat."})
	messageServer.Broadcast(joinMsg)

	// Defer cleanup
	defer func() {
		messageServer.RemoveClient(name)
		messageServer.BroadcastOnlineList()
		leaveMsg, _ := json.Marshal(map[string]string{"type": "system", "message": name + " left the chat."})
		messageServer.Broadcast(leaveMsg)
		log.Printf("Session #%d closed. Client: %s", sessionID, name)
	}()

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(session.Context())
	defer cancel()

	// Goroutine for sending messages from channel to client
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer sendStream.Close()
		for {
			select {
			case msg := <-client.Ch:
				_, err := sendStream.Write(msg)
				if err != nil {
					log.Printf("[%s] Send stream failed: %v", name, err)
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Goroutine for accepting chat messages from client (unidirectional)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			stream, err := session.AcceptUniStream(ctx)
			if err != nil {
				log.Printf("[%s] Stopped accepting chat streams: %v", name, err)
				cancel()
				return
			}
			go handleChatMessage(messageServer, client, stream)
		}
	}()

	// Goroutine for accepting bidirectional streams and routing them
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			stream, err := session.AcceptStream(ctx)
			if err != nil {
				log.Printf("[%s] Stopped accepting bidirectional streams: %v", name, err)
				cancel()
				return
			}
			
			// Route to appropriate handler based on first bytes
			go routeBidirectionalStream(ctx, messageServer, client, stream)
		}
	}()

	wg.Wait()
}

// routeBidirectionalStream reads the first few bytes to determine stream type
func routeBidirectionalStream(ctx context.Context, messageServer *MessageServer, client *Client, stream *webtransport.Stream) {
	defer stream.Close()
	
	// Peek at the first bytes to determine stream type
	// For drawing: starts with 4-byte length (binary)
	// For file: starts with JSON header ending with \n
	
	peekBuf := make([]byte, 8) // Read first 8 bytes
	n, err := stream.Read(peekBuf)
	if err != nil && err != io.EOF {
		log.Printf("[%s] Error peeking stream: %v", client.Name, err)
		return
	}
	
	if n == 0 {
		log.Printf("[%s] Empty stream received", client.Name)
		return
	}
	
	// Check if it looks like drawing (4-byte big-endian length at start)
	// Drawing header length will be small (< 1000 bytes typically)
	if n >= 4 {
		headerLen := uint32(peekBuf[0])<<24 | uint32(peekBuf[1])<<16 | uint32(peekBuf[2])<<8 | uint32(peekBuf[3])
		
		// If header length is reasonable for JSON (20-1000 bytes) and byte 4 might be '{'
		// then it's likely a drawing with length-prefixed header
		if headerLen > 10 && headerLen < 1000 && (n < 5 || peekBuf[4] == '{' || peekBuf[4] == ' ') {
			log.Printf("[%s] Routing to drawing handler (detected length-prefix: %d)", client.Name, headerLen)
			handleDrawingStreamWithPeek(messageServer, client, stream, peekBuf[:n])
			return
		}
	}
	
	// Check if it starts with JSON (file operations)
	if peekBuf[0] == '{' {
		log.Printf("[%s] Routing to file handler (detected JSON)", client.Name)
		handleFileStreamWithPeek(ctx, messageServer, client, stream, peekBuf[:n])
		return
	}
	
	// Default to file handler for backward compatibility
	log.Printf("[%s] Routing to file handler (default)", client.Name)
	handleFileStreamWithPeek(ctx, messageServer, client, stream, peekBuf[:n])
}

// handleDrawingStreamWithPeek handles drawing with already-read peek bytes
func handleDrawingStreamWithPeek(server *MessageServer, client *Client, s *webtransport.Stream, peekData []byte) {
	log.Printf("[%s] Drawing stream started", client.Name)

	// Read header with fixed-length prefix (using peek data)
	hdr, remainingData, err := readDrawingHeaderWithPeek(s, peekData)
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
	
	// First copy remaining data from header read
	if len(remainingData) > 0 {
		copy(imageData, remainingData)
		totalRead = int64(len(remainingData))
	}
	
	// Then read the rest
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

// handleFileStreamWithPeek handles file operations with already-read peek bytes
func handleFileStreamWithPeek(ctx context.Context, server *MessageServer, client *Client, s *webtransport.Stream, peekData []byte) {
	// Create a multi-reader that includes peek data
	reader := io.MultiReader(&bytesReader{data: peekData}, s)
	
	// Read header from combined reader
	hdr, err := readStreamHeaderFromReader(reader)
	if err != nil {
		log.Printf("[%s] Error reading stream header: %v", client.Name, err)
		writeJSONResult(s, map[string]string{"status": "error", "error": err.Error()})
		return
	}

	// Validate not a drawing
	if hdr.Op == "drawing" {
		log.Printf("[%s] Drawing operation sent to file handler - rejecting", client.Name)
		writeJSONResult(s, map[string]string{
			"status": "error", 
			"error":  "invalid operation: use drawing endpoint for drawings",
		})
		return
	}

	hdr.Filename = sanitizeFilename(hdr.Filename)

	// Route to appropriate file handler
	// Create a custom stream wrapper that reads from our reader
	wrappedReader := &readerStream{s: s, r: reader}
	
	switch hdr.Op {
	case "upload":
		handleUploadFromReader(client, s, hdr, wrappedReader)
	case "merge":
		handleMerge(server, client, s, hdr)
	case "download":
		handleDownload(client, s, hdr)
	default:
		log.Printf("[%s] Unknown file operation: %s", client.Name, hdr.Op)
		writeJSONResult(s, map[string]string{"status": "error", "error": "unknown operation"})
	}
}

// readDrawingHeaderWithPeek reads header using peeked data
func readDrawingHeaderWithPeek(s io.Reader, peekData []byte) (*drawingHeader, []byte, error) {
	// We need at least 4 bytes for length
	totalNeeded := 4
	currentData := make([]byte, len(peekData))
	copy(currentData, peekData)
	
	// Read more if needed
	for len(currentData) < totalNeeded {
		tmp := make([]byte, totalNeeded-len(currentData))
		n, err := s.Read(tmp)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read header length: %w", err)
		}
		currentData = append(currentData, tmp[:n]...)
	}
	
	// Parse header length
	headerLength := uint32(currentData[0])<<24 | uint32(currentData[1])<<16 | uint32(currentData[2])<<8 | uint32(currentData[3])
	
	if headerLength == 0 || headerLength > 16*1024 {
		return nil, nil, fmt.Errorf("invalid header length: %d bytes", headerLength)
	}
	
	// Read complete header
	totalNeeded = 4 + int(headerLength)
	for len(currentData) < totalNeeded {
		tmp := make([]byte, totalNeeded-len(currentData))
		n, err := s.Read(tmp)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read header JSON: %w", err)
		}
		currentData = append(currentData, tmp[:n]...)
	}
	
	// Parse JSON header
	headerJSON := currentData[4 : 4+headerLength]
	var hdr drawingHeader
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		return nil, nil, fmt.Errorf("invalid drawing header format: %w", err)
	}
	
	// Return remaining data after header
	remaining := currentData[4+headerLength:]
	return &hdr, remaining, nil
}

// readStreamHeaderFromReader reads file header from a reader
func readStreamHeaderFromReader(r io.Reader) (*fileStreamHeader, error) {
	headerBuf := make([]byte, 0, 16*1024)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			headerBuf = append(headerBuf, tmp[:n]...)
			if i := bytes.IndexByte(headerBuf, '\n'); i >= 0 {
				headerBuf = headerBuf[:i]
				break
			}
		}
		if err != nil {
			return nil, fmt.Errorf("reading header failed: %w", err)
		}
		if len(headerBuf) > 16*1024 {
			return nil, fmt.Errorf("header too large")
		}
	}

	var hdr fileStreamHeader
	if err := json.Unmarshal(headerBuf, &hdr); err != nil {
		return nil, fmt.Errorf("invalid header format: %w", err)
	}
	return &hdr, nil
}

// handleUploadFromReader handles upload with custom reader
func handleUploadFromReader(client *Client, s *webtransport.Stream, hdr *fileStreamHeader, reader io.Reader) {
	tempFile := filepath.Join("uploads", fmt.Sprintf("%s.part%d", hdr.Filename, hdr.ChunkIndex))
	f, err := os.Create(tempFile)
	if err != nil {
		writeJSONResult(s, map[string]string{"status": "error", "error": "cannot create temp file"})
		return
	}
	defer f.Close()

	if hdr.Size > 0 {
		f.Truncate(hdr.Size)
	}

	log.Printf("⬆[%s] Receiving chunk %d for %s (%.2f MB)",
		client.Name, hdr.ChunkIndex, hdr.Filename, float64(hdr.Size)/(1024*1024))

	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	written, err := io.Copy(f, reader)
	if err != nil {
		writeJSONResult(s, map[string]string{"status": "error", "error": "failed to write chunk to disk"})
		return
	}
	f.Sync()

	log.Printf("[%s] Finished receiving chunk %d (%s), %.2f MB written.",
		client.Name, hdr.ChunkIndex, hdr.Filename, float64(written)/(1024*1024))

	writeJSONResult(s, map[string]interface{}{
		"status":      "ok",
		"chunk_index": hdr.ChunkIndex,
		"bytes":       written,
	})
}

// readerStream wraps a reader to use as stream
type readerStream struct {
	s *webtransport.Stream
	r io.Reader
}

func (rs *readerStream) Read(p []byte) (n int, err error) {
	return rs.r.Read(p)
}

// handleChatMessage reads a message from a unidirectional stream and broadcasts it.
func handleChatMessage(messageServer *MessageServer, client *Client, stream *webtransport.ReceiveStream) {
	// Set a deadline for reading to avoid hanging goroutines
	stream.SetReadDeadline(time.Now().Add(10 * time.Minute))

	p, err := io.ReadAll(stream)
	if err != nil {
		log.Printf("[%s] Failed to read from chat stream: %v", client.Name, err)
		return
	}

	var msg map[string]interface{}
	if json.Unmarshal(p, &msg) == nil {
		msg["type"] = "chat"
		msg["name"] = client.Name
		b, _ := json.Marshal(msg)
		messageServer.Broadcast(b)
	}
}

// bytesReader is a simple reader for already-read bytes
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}