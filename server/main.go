package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"

	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

func main() {
	// Use all available CPU cores
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Ensure the 'uploads' directory exists
	if err := os.MkdirAll("uploads", 0o755); err != nil {
		log.Fatalf("Failed to create 'uploads' directory: %v", err)
	}

	// Initialize the central message server
	messageServer := NewMessageServer()

	// Configure the WebTransport server
	wt := webtransport.Server{
		H3: http3.Server{
			Addr: ":4433",
		},
		CheckOrigin: func(r *http.Request) bool {
			// Allow all origins for simplicity
			return true
		},
	}

	// Use an atomic counter for unique session IDs
	var sessionIDCounter int32

	// Define the HTTP handler for the /chat endpoint
	http.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		session, err := wt.Upgrade(w, r)
		if err != nil {
			log.Printf("Upgrading to WebTransport failed: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		sessionID := atomic.AddInt32(&sessionIDCounter, 1)
		go handleWebTransportSession(messageServer, int(sessionID), session, r)
	})

	log.Println("Starting WebTransport chat server on :4433 ...")
	log.Println("File uploads will be saved to ./uploads/")
	log.Printf("Multi-stream mode: %d concurrent streams", NUM_STREAMS)
	log.Printf("Chunk size: %d MB", CHUNK_SIZE/(1024*1024))

	// Start the server (requires certificate and key files)
	// Make sure 'cert.pem' and 'key.pem' are in the same directory or provide the correct path.
	err := wt.ListenAndServeTLS("server.pem", "server-key.pem")
	if err != nil {
		log.Fatalf("ListenAndServeTLS failed: %v", err)
	}
}
