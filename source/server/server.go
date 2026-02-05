package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
)

// MessageServer manages connected clients and broadcasting messages.
type MessageServer struct {
	listeners map[string]*Client
	mutex     sync.Mutex
}

// NewMessageServer creates a new MessageServer instance.
func NewMessageServer() *MessageServer {
	return &MessageServer{
		listeners: make(map[string]*Client),
	}
}

// AddClient registers a new client with the server.
func (m *MessageServer) AddClient(c *Client) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.listeners[c.Name] = c
	log.Printf("Client added: %s. Total clients: %d", c.Name, len(m.listeners))
}

// RemoveClient removes a client by name.
func (m *MessageServer) RemoveClient(name string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if c, ok := m.listeners[name]; ok {
		close(c.Ch)
		delete(m.listeners, name)
		log.Printf("Client removed: %s. Total clients: %d", name, len(m.listeners))
	}
}

// Broadcast sends a message to all connected clients.
func (m *MessageServer) Broadcast(message []byte) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, c := range m.listeners {
		select {
		case c.Ch <- message:
		default:
			log.Printf("[WARN] Channel full for client %s, skipping message.", c.Name)
		}
	}
}

// BroadcastOnlineList sends the list of currently online users to all clients.
func (m *MessageServer) BroadcastOnlineList() {
	m.mutex.Lock()
	names := make([]string, 0, len(m.listeners))
	for name := range m.listeners {
		names = append(names, name)
	}
	m.mutex.Unlock() // Unlock early before marshaling and sending

	data, err := json.Marshal(map[string]interface{}{
		"type":    "online",
		"clients": names,
	})
	if err != nil {
		log.Printf("Error marshaling online list: %v", err)
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	log.Printf("Broadcasting online list to %d clients.", len(m.listeners))
	for _, c := range m.listeners {
		if err := c.Session.SendDatagram(data); err != nil {
			log.Printf("Failed to send online list to %s: %v", c.Name, err)
		}
	}
}

// BroadcastFileList sends the list of available files to all clients.
func (m *MessageServer) BroadcastFileList() {
	fileList := getFileList()
	data, err := json.Marshal(map[string]interface{}{
		"type":  "file_list",
		"files": fileList,
	})
	if err != nil {
		log.Printf("Error marshaling file list: %v", err)
		return
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	log.Printf("Broadcasting file list (%d files) to %d clients.", len(fileList), len(m.listeners))
	for _, c := range m.listeners {
		if err := c.Session.SendDatagram(data); err != nil {
			log.Printf("Failed to send file list to %s: %v", c.Name, err)
		}
	}
}

// SendFileList sends the file list to a single, specific client.
func (m *MessageServer) SendFileList(c *Client) {
	fileList := getFileList()
	data, err := json.Marshal(map[string]interface{}{
		"type":  "file_list",
		"files": fileList,
	})
	if err != nil {
		log.Printf("Error marshaling file list for %s: %v", c.Name, err)
		return
	}

	if err := c.Session.SendDatagram(data); err != nil {
		log.Printf("Failed to send file list to %s: %v", c.Name, err)
	} else {
		log.Printf("Sent file list (%d files) to %s.", len(fileList), c.Name)
	}
}

// getFileList reads the 'uploads' directory and returns a list of files.
func getFileList() []map[string]interface{} {
	files, err := os.ReadDir("uploads")
	if err != nil {
		log.Printf("Error reading 'uploads' directory: %v", err)
		return []map[string]interface{}{}
	}

	var fileList []map[string]interface{}
	for _, f := range files {
		// Skip directories and temporary/hidden files
		if f.IsDir() || strings.HasPrefix(f.Name(), ".") || strings.HasSuffix(f.Name(), ".tmp") || strings.Contains(f.Name(), ".part") {
			continue
		}

		info, err := f.Info()
		if err != nil {
			continue
		}

		fileList = append(fileList, map[string]interface{}{
			"name": info.Name(),
			"size": info.Size(),
		})
	}
	return fileList
}
