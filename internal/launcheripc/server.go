package launcheripc

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type CoordinatorHandler interface {
	HandleConnect(ctx context.Context, req *ConnectRequest) (*ConnectResponseData, error)
	HandleUpdateApply(ctx context.Context, req *UpdateApplyRequest) (*UpdateAccepted, error)
	HandleAppReady(ctx context.Context, req *AppReadyRequest) error
	HandleServerHealthy(ctx context.Context, req *ServerHealthyRequest) error
}

type Server struct {
	endpoint    string
	token       string
	expectedUID uint32
	coordinator CoordinatorHandler
	listener    net.Listener
	httpServer  *http.Server
	cleanup     func() error
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
	mux.HandleFunc("/v1/updates/apply", s.handleUpdateApply)
	mux.HandleFunc("/v1/session/app-ready", s.handleAppReady)
	mux.HandleFunc("/v1/session/server-healthy", s.handleServerHealthy)

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

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, ErrInvalidRequest, "method not allowed")
		return
	}
	var req UpdateApplyRequest
	if err := ReadRequestJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	accepted, err := s.coordinator.HandleUpdateApply(r.Context(), &req)
	if err != nil {
		if strings.Contains(err.Error(), "in progress") {
			s.writeError(w, http.StatusConflict, ErrUpdateInProgress, err.Error())
			return
		}
		s.writeError(w, http.StatusBadRequest, ErrInvalidRequest, err.Error())
		return
	}
	s.writeSuccess(w, http.StatusOK, accepted)
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
