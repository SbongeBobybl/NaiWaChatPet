package aiorganization

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	winio "github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

type Descriptor struct {
	SchemaVersion int    `json:"schemaVersion"`
	PipeName      string `json:"pipeName"`
	Session       string `json:"session"`
	Token         string `json:"token"`
	PID           int    `json:"pid"`
}
type Server struct {
	listener    net.Listener
	descriptor  Descriptor
	sid         string
	sessionID   uint32
	handler     Handler
	mu          sync.Mutex
	requests    map[string]context.CancelFunc
	connections map[net.Conn]bool
	slots       chan struct{}
	closed      bool
}

func currentSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}
func randomID() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func privateDirectory(path, sid string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe agent directory")
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// Start publishes a short-lived handshake in an explicitly user-only directory.
// The kernel rejects remote pipes; every accepted client is also checked by SID/session.
func Start(directory string, handler Handler) (*Server, error) {
	sid, err := currentSID()
	if err != nil {
		return nil, err
	}
	var sessionID uint32
	if err = windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		return nil, err
	}
	if err = privateDirectory(directory, sid); err != nil {
		return nil, err
	}
	session, err := randomID()
	if err != nil {
		return nil, err
	}
	token, err := randomID()
	if err != nil {
		return nil, err
	}
	// The random session is part of the pipe name: a stale or second instance
	// can never share the name of a fresh server, so clients always reach the
	// instance their descriptor was published by.
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", sid, sessionID, session)))
	name := fmt.Sprintf("NaiWa.DeskBox.v1.%x", hash[:12])
	l, err := winio.ListenPipe(`\\.\pipe\`+name, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")", InputBufferSize: MaxMessage, OutputBufferSize: MaxMessage})
	if err != nil {
		return nil, err
	}
	s := &Server{listener: l, sid: sid, sessionID: sessionID, handler: handler, requests: map[string]context.CancelFunc{}, connections: map[net.Conn]bool{}, slots: make(chan struct{}, 8), descriptor: Descriptor{Version, name, session, token, os.Getpid()}}
	body, err := json.Marshal(s.descriptor)
	if err != nil {
		l.Close()
		return nil, err
	}
	// A new file inherits the directory DACL before any token bytes are written.
	tmp := filepath.Join(directory, "agent-"+session+".tmp")
	if err = os.WriteFile(tmp, body, 0600); err == nil {
		err = os.Rename(tmp, filepath.Join(directory, "agent.json"))
	}
	if err != nil {
		l.Close()
		os.Remove(tmp)
		return nil, err
	}
	go s.accept()
	return s, nil
}
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	for _, cancel := range s.requests {
		cancel()
	}
	for c := range s.connections {
		c.Close()
	}
	s.mu.Unlock()
	return s.listener.Close()
}
func (s *Server) accept() {
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
		default:
			c.Close()
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			<-s.slots
			return
		}
		s.connections[c] = true
		s.mu.Unlock()
		go func() {
			defer func() { c.Close(); s.mu.Lock(); delete(s.connections, c); s.mu.Unlock(); <-s.slots }()
			s.serve(c)
		}()
	}
}

var getClientPID = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeClientProcessId")

func (s *Server) verifyClient(c net.Conn) bool {
	f, ok := c.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	var pid uint32
	r, _, _ := getClientPID.Call(f.Fd(), uintptr(unsafe.Pointer(&pid)))
	if r == 0 {
		return false
	}
	var session uint32
	if windows.ProcessIdToSessionId(pid, &session) != nil || session != s.sessionID {
		return false
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(p)
	var token windows.Token
	if windows.OpenProcessToken(p, windows.TOKEN_QUERY, &token) != nil {
		return false
	}
	defer token.Close()
	u, err := token.GetTokenUser()
	return err == nil && u.User.Sid.String() == s.sid
}
func (s *Server) serve(c net.Conn) {
	if !s.verifyClient(c) {
		return
	}
	c.SetDeadline(time.Now().Add(35 * time.Second))
	var req Request
	if ReadFrame(c, &req) != nil {
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(s.descriptor.Token)) != 1 || req.Session != s.descriptor.Session {
		return
	}
	res := Response{SchemaVersion: Version, RequestID: req.RequestID, Session: s.descriptor.Session, Status: "ok", Configured: s.handler.Configured()}
	if req.Validate() != nil {
		res.Status = "invalid"
		WriteFrame(c, res)
		return
	}
	switch req.Operation {
	case "health":
	case "complete_carry":
		h, ok := s.handler.(CarryHandler)
		if !ok {
			res.Status = "unavailable"
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if h.CompleteCarry(ctx, req) != nil {
			res.Status = "unavailable"
		}
	case "cancel":
		s.mu.Lock()
		cancel := s.requests[req.CancelRequestID]
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		} else if h, ok := s.handler.(CarryHandler); ok {
			ctx, stop := context.WithTimeout(context.Background(), 12*time.Second)
			_ = h.CompleteCarry(ctx, Request{CarryRequestID: req.CancelRequestID, Succeeded: false})
			stop()
		}
	default:
		if !req.Consent {
			res.Status = "consent_required"
			break
		}
		duration := 80 * time.Second
		c.SetDeadline(time.Now().Add(90 * time.Second))
		if req.Operation == "carry" {
			duration = 170 * time.Second
			c.SetDeadline(time.Now().Add(180 * time.Second))
		}
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()
		s.mu.Lock()
		busy := len(s.requests) > 0 || s.closed
		if !busy {
			s.requests[req.RequestID] = cancel
		}
		s.mu.Unlock()
		if busy {
			res.Status = "busy"
			break
		}
		defer func() { s.mu.Lock(); delete(s.requests, req.RequestID); s.mu.Unlock() }()
		var result Result
		var err error
		if req.Operation == "carry" {
			h, ok := s.handler.(CarryHandler)
			if !ok {
				res.Status = "unavailable"
				break
			}
			err = h.Carry(ctx, req)
			if err == nil {
				res.Status = "arrived"
				break
			}
		} else if req.Operation == "classify" {
			result, err = s.handler.Classify(ctx, req)
		} else {
			result, err = s.handler.ParseIntent(ctx, req)
		}
		if err == nil {
			err = ValidateResult(req, result)
		}
		if err != nil {
			res.Error = err.Error()
			if len(res.Error) > 240 {
				res.Error = res.Error[:240]
			}
			res.Status = "offline"
			if errors.Is(err, ErrInvalid) {
				res.Status = "invalid_result"
			}
			if ctx.Err() != nil {
				res.Status = "cancelled"
			}
		} else {
			res.Result = &result
		}
	}
	WriteFrame(c, res)
}
