package main

import (
	"context"
	"encoding/json"
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
			// FIX 1: Pass the ADDRESS of the stream (&stream) so it's a pointer.
			go handleChatMessage(messageServer, client, stream)
		}
	}()

	// Goroutine for accepting bidirectional file streams from client
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			stream, err := session.AcceptStream(ctx)
			if err != nil {
				log.Printf("[%s] Stopped accepting file streams: %v", name, err)
				cancel()
				return
			}
			go handleFileStream(ctx, messageServer, client, stream)
		}
	}()

	wg.Wait()
}

// handleChatMessage reads a message from a unidirectional stream and broadcasts it.
// FIX 2: Change the function signature to accept a POINTER (*webtransport.ReceiveStream).
func handleChatMessage(messageServer *MessageServer, client *Client, stream *webtransport.ReceiveStream) {
	// Set a deadline for reading to avoid hanging goroutines
	stream.SetReadDeadline(time.Now().Add(10 * time.Minute))

	// Now that 'stream' is a pointer, it satisfies the io.Reader interface and can be used here.
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
