// Package sse 提供 SSE 流式输出工具。
package sse

import (
	"fmt"
	"io"
	"net/http"
)

// Writer 封装一个 SSE 写端。
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// New 构造 SSE Writer 并写入必要 header。
func New(w http.ResponseWriter) (*Writer, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("sse: response writer does not support flushing")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 禁用 nginx 缓冲
	return &Writer{w: w, flusher: flusher}, nil
}

// WriteData 写一条 data 事件。
func (s *Writer) WriteData(data []byte) error {
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteDone 写结束标记。
func (s *Writer) WriteDone() error {
	if _, err := io.WriteString(s.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
