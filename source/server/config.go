package main

import (
	"sync"
)

const (
	CHUNK_SIZE = 16 << 20 // 16MB

	NUM_STREAMS = 8
)

// during file transfers. Each buffer is CHUNK_SIZE.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, CHUNK_SIZE)
		return &buf
	},
}
