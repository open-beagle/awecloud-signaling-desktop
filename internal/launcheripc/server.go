package launcheripc

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CoordinatorHandler interface {
	HandleConnect(ctx context.Context, req *ConnectRequest) (*ConnectResponseData, error)
	HandleUpdateRequest(ctx context.Context, req *UpdateRequest) (*UpdateSnapshot, error)
	HandleUpdateConfirm(ctx context.Context, req *UpdateConfirmRequest) error
	HandleAppReady(ctx context.Context, req *AppReadyRequest) error
	HandleServerHealthy(ctx context.Context, req *ServerHealthyRequest) error
	GetState(ctx context.Context) (*StateResponseData, error)
	GetEvents(ctx context.Context, afterSeq int64, waitSec int) (*EventsResponseData, error)
}

type Server struct {
	endpoint    string
	token       string
	expectedUID uint32
	coordinator CoordinatorHandler
	listener    net.Listener
	httpServer  *http.Server
	cleanup     func() error
	pollMu      sync.Mutex
	hasActivePoll bool
}

func NewServer(endpoint, token string, expectedUID uint32, handler CoordinatorHandler) *Server {
	return &Server{
		endpoint:    endpoint,
		token:       token,
		expectedUID: expectedUID,
		coordinator: handler,
	}
}

func (s *Server) Start() error {
	listener, cleanup, err := listenLocal(s.endpoint)
	if err != nil {
		return err
	}
	s.listener = listener
	s.cleanup = cleanup

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/session/connect", s.handleConnect)
	mux.HandleFunc("/v1/updates/request", s.handleUpdateRequest)
	mux.HandleFunc("/v1/updates/confirm", s.handleUpdateConfirm)
	mux.HandleFunc("/v1/session/app-ready", s.handleAppReady)
	mux.HandleFunc("/v1/session/server-healthy", s.handleServerHealthy)
	mux.HandleFunc("/v1/state", s.handleState)
	mux.HandleFunc("/v1/events", s.handleEvents)

	handler := s.authMiddleware(s.browserCheckMiddleware(mux))

	s.httpServer = &http.Server{
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		_ = s.httpServer.Serve(s.listener)
	}()

	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(ctx)
	}
	if s.cleanup != nil {
		_ = s.cleanup()
	}
	return nil
}

func (s *Server) browserCheckMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" || r.Header.Get("Access-Control-Request-Method") != "" {
			s.writeError(w, http.StatusForbidden, ErrInvalidRequest, "browser requests forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			s.writeError(w, http.StatusUnauthorized, ErrSessionMismatch, "invalid authorization token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	var req ConnectRequest
	if err := ReadRequestJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	if req.SchemaVersion != SchemaVersion {
		s.writeError(w, http.StatusBadRequest, ErrUnsupportedSchema, "unsupported schema_version")
		return
	}
	if req.IPCVersion != IPCVersion {
		s.writeError(w, http.StatusBadRequest, ErrUnsupportedIPCVersion, "unsupported ipc_version")
		return
	}

	data, err := s.coordinator.HandleConnect(r.Context(), &req)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, data)
}

func (s *Server) handleUpdateRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	var req UpdateRequest
	if err := ReadRequestJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	snapshot, err := s.coordinator.HandleUpdateRequest(r.Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "in progress") {
			s.writeError(w, http.StatusConflict, ErrUpdateInProgress, err.Error())
			return
		}
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, map[string]any{
		"operation_id": snapshot.OperationID,
		"phase":        snapshot.Phase,
		"duplicate":    false,
	})
}

func (s *Server) handleUpdateConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	var req UpdateConfirmRequest
	if err := ReadRequestJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	if err := s.coordinator.HandleUpdateConfirm(r.Context(), &req); err != nil {
		s.writeError(w, http.StatusConflict, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleAppReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	var req AppReadyRequest
	if err := ReadRequestJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	if err := s.coordinator.HandleAppReady(r.Context(), &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleServerHealthy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	var req ServerHealthyRequest
	if err := ReadRequestJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	if err := s.coordinator.HandleServerHealthy(r.Context(), &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, nil)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	data, err := s.coordinator.GetState(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, data)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}

	s.pollMu.Lock()
	if s.hasActivePoll {
		s.pollMu.Unlock()
		s.writeError(w, http.StatusConflict, ErrConcurrentPoll, "another poll is already active")
		return
	}
	s.hasActivePoll = true
	s.pollMu.Unlock()

	defer func() {
		s.pollMu.Lock()
		s.hasActivePoll = false
		s.pollMu.Unlock()
	}()

	afterSeq, _ := strconv.ParseInt(r.URL.Query().Get("after_sequence"), 10, 64)
	waitSec, _ := strconv.Atoi(r.URL.Query().Get("wait_seconds"))
	if waitSec < 0 {
		waitSec = 0
	}
	if waitSec > 25 {
		waitSec = 25
	}

	data, err := s.coordinator.GetEvents(r.Context(), afterSeq, waitSec)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, data)
}

func (s *Server) writeSuccess(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(CommonResponse{
		SchemaVersion: SchemaVersion,
		OK:            true,
		Data:          data,
	})
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(CommonResponse{
		SchemaVersion: SchemaVersion,
		OK:            false,
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

var _ = os.Getenv
