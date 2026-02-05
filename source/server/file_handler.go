package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/quic-go/webtransport-go"
)

// fileStreamHeader defines the structure of the JSON header received on a file stream.
type fileStreamHeader struct {
	Op         string `json:"op"`
	Filename   string `json:"filename"`
	Size       int64  `json:"size,omitempty"`
	Hash       string `json:"hash,omitempty"`
	ChunkIndex int    `json:"chunk_index,omitempty"`
	ChunkStart int64  `json:"chunk_start,omitempty"`
	ChunkEnd   int64  `json:"chunk_end,omitempty"`
}

// handleUpload handles upload with custom reader
func handleUpload(client *Client, s *webtransport.Stream, hdr *fileStreamHeader, reader io.Reader) {
	tempFile := filepath.Join("uploads", fmt.Sprintf("%s.part%d", hdr.Filename, hdr.ChunkIndex))
	f, err := os.Create(tempFile)
	if err != nil {
		writeJSONResult(s, map[string]string{"status": "error", "error": "cannot create temp file"})
		return
	}
	defer f.Close()

	if hdr.Size > 0 {
		f.Truncate(hdr.Size) // Pre-allocate file size to reduce fragmentation
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

// handleMerge combines chunks into a final file and verifies it.
func handleMerge(server *MessageServer, client *Client, s *webtransport.Stream, hdr *fileStreamHeader) {
	log.Printf("[%s] Starting merge for %s", client.Name, hdr.Filename)
	finalFile := filepath.Join("uploads", hdr.Filename)
	f, err := os.Create(finalFile)
	if err != nil {
		writeJSONResult(s, map[string]string{"status": "error", "error": "cannot create final file"})
		return
	}
	defer f.Close()

	h := sha256.New()
	multiWriter := io.MultiWriter(f, h)
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	var totalBytes int64
	for i := 0; i < NUM_STREAMS; i++ {
		partFile := filepath.Join("uploads", fmt.Sprintf("%s.part%d", hdr.Filename, i))
		pf, err := os.Open(partFile)
		if err != nil {
			log.Printf("[%s] Missing part %d for %s", client.Name, i, hdr.Filename)
			writeJSONResult(s, map[string]string{"status": "error", "error": fmt.Sprintf("missing part %d", i)})
			os.Remove(finalFile) // Clean up failed merge
			return
		}

		written, err := io.CopyBuffer(multiWriter, pf, *bufPtr)
		pf.Close()
		os.Remove(partFile)

		if err != nil {
			writeJSONResult(s, map[string]string{"status": "error", "error": "failed during merge copy"})
			os.Remove(finalFile) // Clean up failed merge
			return
		}
		totalBytes += written
	}
	f.Sync()

	// Verify hash if provided
	if hdr.Hash != "" {
		calculatedHash := fmt.Sprintf("%x", h.Sum(nil))
		if !strings.EqualFold(calculatedHash, hdr.Hash) {
			os.Remove(finalFile)
			log.Printf("[%s] Hash mismatch for %s. Expected: %s, Got: %s", client.Name, hdr.Filename, hdr.Hash, calculatedHash)
			writeJSONResult(s, map[string]string{"status": "error", "error": "file hash mismatch"})
			return
		}
		log.Printf("[%s] Hash matched for %s", client.Name, hdr.Filename)
	}

	log.Printf("[%s] Merge complete: %s (%.2f MB)", client.Name, hdr.Filename, float64(totalBytes)/(1024*1024))
	writeJSONResult(s, map[string]interface{}{"status": "ok", "filename": hdr.Filename, "bytes": totalBytes})

	// Notify all clients of the new file
	go func() {
		server.BroadcastFileList()
		msg, _ := json.Marshal(map[string]interface{}{
			"type": "file", "name": client.Name, "filename": hdr.Filename, "size": totalBytes,
		})
		server.Broadcast(msg)
	}()
}

// handleDownload processes a request to download a file chunk.
func handleDownload(client *Client, s *webtransport.Stream, hdr *fileStreamHeader) {
	fpath := filepath.Join("uploads", hdr.Filename)
	f, err := os.Open(fpath)
	if err != nil {
		writeJSONResult(s, map[string]string{"status": "error", "error": "file not found"})
		return
	}
	defer f.Close()

	info, _ := f.Stat()
	fileSize := info.Size()

	// Handle initial metadata request
	if hdr.ChunkIndex == -1 {
		log.Printf("[%s] Sending metadata for %s (%.2f MB)", client.Name, hdr.Filename, float64(fileSize)/(1024*1024))
		writeJSONResult(s, map[string]interface{}{
			"status": "ok", "filename": hdr.Filename, "size": fileSize, "num_streams": NUM_STREAMS,
		})
		return
	}

	// Send the requested chunk
	chunkSize := hdr.ChunkEnd - hdr.ChunkStart
	log.Printf("[%s] Sending chunk %d of %s (%.2f MB)",
		client.Name, hdr.ChunkIndex, hdr.Filename, float64(chunkSize)/(1024*1024))

	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	sectionReader := io.NewSectionReader(f, hdr.ChunkStart, chunkSize)
	sent, err := io.CopyBuffer(s, sectionReader, *bufPtr)
	if err != nil {
		log.Printf("[%s] Error sending chunk %d: %v", client.Name, hdr.ChunkIndex, err)
		return
	}

	log.Printf("[%s] Finished sending chunk %d: %.2f MB", client.Name, hdr.ChunkIndex, float64(sent)/(1024*1024))
}

// writeJSONResult marshals a struct to JSON and writes it to the stream.
func writeJSONResult(w io.Writer, v interface{}) {
	b, _ := json.Marshal(v)
	b = append(b, '\n') // Use newline as a delimiter
	w.Write(b)
}

// sanitizeFilename cleans a filename to prevent path traversal attacks.
func sanitizeFilename(name string) string {
	return strings.ReplaceAll(filepath.Base(name), "..", "")
}
