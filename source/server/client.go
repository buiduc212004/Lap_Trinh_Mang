package main

import (
	"github.com/quic-go/webtransport-go"
)

/**
 * Cấu trúc đại diện cho một client kết nối
 */
type Client struct {
	Name    string                
	Session *webtransport.Session 
	Ch      chan []byte           

	SendStream *webtransport.SendStream
}
