package gatesentryproxy

import (
	"bytes"
	"sync"
)

// Buffer pools to reduce memory allocations
var (
	// Pool for small buffers (up to 4KB) - used for auth headers, small data
	smallBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 4096)
			return &buf
		},
	}

	// Pool for medium buffers (up to 64KB) - used for typical responses
	mediumBufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	// Pool for large buffers (up to 2MB) - used for large content scanning
	largeBufferPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, 2*1024*1024)
			return &buf
		},
	}
)

// GetSmallBuffer returns a small buffer from the pool
func GetSmallBuffer() *[]byte {
	return smallBufferPool.Get().(*[]byte)
}

// PutSmallBuffer returns a small buffer to the pool
func PutSmallBuffer(buf *[]byte) {
	if buf != nil {
		smallBufferPool.Put(buf)
	}
}

// GetMediumBuffer returns a medium buffer from the pool
func GetMediumBuffer() *bytes.Buffer {
	buf := mediumBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutMediumBuffer returns a medium buffer to the pool
func PutMediumBuffer(buf *bytes.Buffer) {
	if buf != nil {
		buf.Reset()
		mediumBufferPool.Put(buf)
	}
}

// GetLargeBuffer returns a large buffer from the pool
func GetLargeBuffer() *[]byte {
	return largeBufferPool.Get().(*[]byte)
}

// PutLargeBuffer returns a large buffer to the pool
func PutLargeBuffer(buf *[]byte) {
	if buf != nil {
		largeBufferPool.Put(buf)
	}
}
