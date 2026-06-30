package supervisor

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

type Request struct {
	Cmd string `json:"cmd"` // "status" (TODO: "reload", "stop")
}

type Response struct {
	OK    bool   `json:"ok"`
	PID   int    `json:"pid,omitempty"` // supervisor pid
	Error string `json:"error,omitempty"`
}

const connTimeout = 5 * time.Second
const maxControlMsg = 4 << 10 // 4 KB

func listenControl(path string) net.Listener {
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "supervisor: control socket listen %s: %v\n", path, err)
		return nil
	}

	_ = os.Chmod(path, 0o600)

	return ln
}

func serveControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // shutdown
		}

		go handleControlConn(conn)
	}
}

func handleControlConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(connTimeout))

	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, maxControlMsg)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: "bad request"})
		return
	}
	_ = json.NewEncoder(conn).Encode(dispatchControl(req))
}

func dispatchControl(req Request) Response {
	switch req.Cmd {
	case "status":
		return Response{OK: true, PID: os.Getpid()}
	default:
		return Response{OK: false, Error: "unknown command: " + req.Cmd}
	}
}

func SupervisorRequest(socketPath string, req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(connTimeout))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}

	var resp Response
	if err := json.NewDecoder(io.LimitReader(conn, maxControlMsg)).Decode(&resp); err != nil {
		return Response{}, err
	}

	return resp, nil
}
