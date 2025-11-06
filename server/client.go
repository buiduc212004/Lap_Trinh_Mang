package main

import (
	"github.com/quic-go/webtransport-go"
)

/**
 * Cấu trúc đại diện cho một client kết nối
 */
type Client struct {
	Name    string                // Tên của client
	Session *webtransport.Session // Phiên kết nối WebTransport
	Ch      chan []byte           // Kênh truyền tin nhắn

	SendStream *webtransport.SendStream // Stream gửi tin vĩnh viễn từ Server -> Client
}
