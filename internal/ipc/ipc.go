package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

const SocketPath = "/tmp/elite-clipboard.sock"

type Request struct {
	Action      string  `json:"action"`
	Query       string  `json:"query,omitempty"`
	ID          int64   `json:"id,omitempty"`
	WorkspaceID *int    `json:"workspace_id,omitempty"`
	PinnedOnly  bool    `json:"pinned_only,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	Tags        string  `json:"tags,omitempty"`
}

type Response struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

func OK(data interface{}) Response {
	return Response{OK: true, Data: data}
}

func Err(err error) Response {
	return Response{OK: false, Error: err.Error()}
}

type Handler func(req Request) Response

type Server struct {
	handler Handler
}

func NewServer(h Handler) *Server {
	return &Server{handler: h}
}

func (s *Server) Listen() error {
	os.Remove(SocketPath)
	ln, err := net.Listen("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("ipc listen: %w", err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		json.NewEncoder(conn).Encode(Err(err))
		return
	}
	resp := s.handler(req)
	json.NewEncoder(conn).Encode(resp)
}

func Send(req Request) (Response, error) {
	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		return Response{}, fmt.Errorf("ipc dial: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}
