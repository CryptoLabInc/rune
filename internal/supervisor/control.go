package supervisor

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// Control protocol: a one-shot newline-delimited JSON request/response over a
// unix socket the running supervisor listens on. Currently read-only (status);
// "reload"/"stop" land with the runed in-place-update step.
type Request struct {
	Cmd string `json:"cmd"` // "status" (later: "reload", "stop")
}

type Response struct {
	OK    bool   `json:"ok"`
	PID   int    `json:"pid,omitempty"` // the supervisor process pid
	Error string `json:"error,omitempty"`
}

const controlConnTimeout = 5 * time.Second

// maxControlMsg caps a single request/response. The protocol is a tiny {cmd}
// object, so this bounds the memory a peer can make the decoder buffer (the
// connection deadline alone bounds time, not bytes).
const maxControlMsg = 4 << 10 // 4 KiB

// listenControl binds the control socket, clearing a stale file first. The
// supervisor lock (held by the caller) guarantees no other live supervisor, so
// removing a leftover socket is safe. Returns nil (logged) on failure —
// supervision must not fail just because the control channel couldn't bind.
//
// Security boundary: the channel is protected by the 0700 ~/.runed directory
// (created by EnsureDirs / acquireSupervisorLock), which only the owning uid
// can traverse — a connect needs that traverse regardless of the socket node's
// own mode. The chmod 0600 below is best-effort defense-in-depth (the node is
// briefly born with the process umask before it). Before step 4 adds
// state-mutating verbs (reload/stop), keep the 0700 dir as the real boundary.
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

// serveControl accepts connections until ln is closed (which happens when the
// watcher returns and its deferred ln.Close() runs). Handlers are detached;
// the read-only status verb is safe to abandon on shutdown. When step 4 adds
// state-mutating verbs (reload/stop), drain in-flight handlers before teardown.
func serveControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed on shutdown
		}
		go handleControlConn(conn)
	}
}

func handleControlConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(controlConnTimeout))

	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, maxControlMsg)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: "bad request"})
		return
	}
	_ = json.NewEncoder(conn).Encode(dispatchControl(req))
}

// dispatchControl handles one request. os.Getpid() here is the supervisor
// process (handlers run in the supervisor), with no shared mutable state — the
// reload/stop verbs that touch the watch loop arrive in a later step.
func dispatchControl(req Request) Response {
	switch req.Cmd {
	case "status":
		return Response{OK: true, PID: os.Getpid()}
	default:
		return Response{OK: false, Error: "unknown command: " + req.Cmd}
	}
}

// SupervisorRequest dials the control socket, sends req, and returns the
// response. A dial error means no supervisor is currently listening.
func SupervisorRequest(socketPath string, req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(controlConnTimeout))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(io.LimitReader(conn, maxControlMsg)).Decode(&resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}
