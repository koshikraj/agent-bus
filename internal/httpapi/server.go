// Package httpapi exposes the bus over a small authenticated JSON API. It is
// the universal entry point: the MCP bridge, the CLI and any remote agent
// with a token all talk to the bus through it.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentbus/internal/bus"
)

const maxBody = 2 << 20

type server struct {
	b   *bus.Bus
	log *slog.Logger
}

func New(b *bus.Bus, log *slog.Logger) http.Handler {
	s := &server{b: b, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /v1/me", s.auth(s.me))
	mux.HandleFunc("GET /v1/agents", s.auth(s.agents))
	mux.HandleFunc("POST /v1/messages", s.auth(s.send))
	mux.HandleFunc("GET /v1/inbox", s.auth(s.inbox))
	mux.HandleFunc("GET /v1/threads/{id}", s.auth(s.thread))
	return mux
}

type handler func(http.ResponseWriter, *http.Request, bus.Agent)

func (s *server) auth(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		a, err := s.b.Authenticate(r.Context(), strings.TrimSpace(token))
		if err != nil {
			s.fail(w, err)
			return
		}
		h(w, r, a)
	}
}

func (s *server) me(w http.ResponseWriter, r *http.Request, a bus.Agent) {
	writeJSON(w, http.StatusOK, a)
}

func (s *server) agents(w http.ResponseWriter, r *http.Request, _ bus.Agent) {
	agents, err := s.b.Agents(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

func (s *server) send(w http.ResponseWriter, r *http.Request, a bus.Agent) {
	var in bus.SendInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&in); err != nil {
		s.fail(w, errors.Join(bus.ErrInvalid, err))
		return
	}
	m, err := s.b.Send(r.Context(), a.Name, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *server) inbox(w http.ResponseWriter, r *http.Request, a bus.Agent) {
	var wait time.Duration
	if v := r.URL.Query().Get("wait"); v != "" {
		secs, err := strconv.ParseFloat(v, 64)
		if err != nil || secs < 0 {
			s.fail(w, errors.Join(bus.ErrInvalid, errors.New("wait must be a non-negative number of seconds")))
			return
		}
		wait = time.Duration(secs * float64(time.Second))
	}
	msgs, err := s.b.Receive(r.Context(), a.Name, r.URL.Query().Get("thread_id"), wait)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *server) thread(w http.ResponseWriter, r *http.Request, a bus.Agent) {
	msgs, err := s.b.Thread(r.Context(), r.PathValue("id"), a.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *server) fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, bus.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, bus.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, bus.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, bus.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, context.Canceled):
		return // client went away
	}
	msg := err.Error()
	if status == http.StatusInternalServerError {
		s.log.Error("request failed", "err", err)
		msg = "internal error"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
