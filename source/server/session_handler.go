package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
		Ch:         make(chan []byte, 256),
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

	// Goroutine for accepting chat messages from client
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

	if n >= 4 {
		headerLen := uint32(peekBuf[0])<<24 | uint32(peekBuf[1])<<16 | uint32(peekBuf[2])<<8 | uint32(peekBuf[3])

		if headerLen > 10 && headerLen < 1000 && (n < 5 || peekBuf[4] == '{' || peekBuf[4] == ' ') {
			log.Printf("[%s] Routing to drawing handler (detected length-prefix: %d)", client.Name, headerLen)
			handleDrawingStreamWithPeek(messageServer, client, stream, peekBuf[:n])
			return
		}
	}

	// Check if it starts with JSON
	if peekBuf[0] == '{' {
		log.Printf("[%s] Routing to file handler (detected JSON)", client.Name)
		handleFileStreamWithPeek(ctx, messageServer, client, stream, peekBuf[:n])
		return
	}

	// Default to file handler for backward compatibility
	log.Printf("[%s] Routing to file handler (default)", client.Name)
	handleFileStreamWithPeek(ctx, messageServer, client, stream, peekBuf[:n])
}

// handleFileStreamWithPeek handles file operations with already-read peek bytes
func handleFileStreamWithPeek(_ context.Context, server *MessageServer, client *Client, s *webtransport.Stream, peekData []byte) {
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

	wrappedReader := &readerStream{s: s, r: reader}

	switch hdr.Op {
	case "upload":
		handleUpload(client, s, hdr, wrappedReader)
	case "merge":
		handleMerge(server, client, s, hdr)
	case "download":
		handleDownload(client, s, hdr)
	default:
		log.Printf("[%s] Unknown file operation: %s", client.Name, hdr.Op)
		writeJSONResult(s, map[string]string{"status": "error", "error": "unknown operation"})
	}
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
