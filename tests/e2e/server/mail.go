package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sync"

	"vanillapay/internal/platform"
)

// Runtime-only inbox for the loopback E2E server. No SMTP and no saved secrets.
type browserInbox struct {
	mu    sync.Mutex
	codes map[string]string
}

func (m *browserInbox) Send(_ context.Context, message platform.MailMessage) error {
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(message.Text)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes[message.To] = code
	return nil
}

func (m *browserInbox) register(mux *http.ServeMux) {
	mux.HandleFunc("POST /__e2e/mail", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Email string `json:"email"`
		}
		if platform.DecodeJSONBody(w, r, 1024, &input) != nil {
			http.Error(w, "invalid mailbox", http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		code := m.codes[input.Email]
		delete(m.codes, input.Email)
		m.mu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
	})
}
