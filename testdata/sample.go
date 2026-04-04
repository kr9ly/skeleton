package sample

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// MaxRetries はリトライ最大回数。
const MaxRetries = 3

var DefaultTimeout = 30

// Status はステータスを表す列挙型。
type Status int

const (
	StatusPending Status = iota
	StatusActive
	StatusDone
)

// Config はアプリケーション設定。
type Config struct {
	Host    string
	Port    int
	Debug   bool
	handler http.Handler // unexported
}

// Server はHTTPサーバーを表す。
type Server struct {
	config Config
	mux    *http.ServeMux
}

// NewServer は Server を生成する。
func NewServer(cfg Config) *Server {
	return &Server{config: cfg, mux: http.NewServeMux()}
}

// Start はサーバーを起動する。
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	return http.ListenAndServe(addr, s.mux)
}

// Shutdown はサーバーを停止する。
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Handler はリクエストを処理するインターフェース。
type Handler interface {
	Handle(ctx context.Context, r *http.Request) error
	Close() error
}

// Reader はデータを読み込むインターフェース。
type Reader interface {
	io.Reader
	ReadAt(p []byte, off int64) (n int, err error)
}

// helper は非公開関数。
func helper() {}

// FormatStatus はステータスを文字列に変換する。
func FormatStatus(s Status) string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusActive:
		return "active"
	case StatusDone:
		return "done"
	default:
		return "unknown"
	}
}

// Middleware は HTTP ミドルウェアの型エイリアス。
type Middleware = func(http.Handler) http.Handler

// HandlerFunc はハンドラ関数型。
type HandlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request) error
