package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"moonflux/internal/user"
	"net/http"
)

type HTTPHandler struct {
	loginService *LoginService
}

func NewHTTPHandler(loginService *LoginService) *HTTPHandler {
	return &HTTPHandler{loginService: loginService}
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var input user.LoginInput
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result, err := h.loginService.Login(r.Context(), input)
	if err != nil {
		if errors.Is(err, user.ErrInvalidCredentials) {
			http.Error(w, "invalid login or password", http.StatusUnauthorized)
			return
		}

		http.Error(w, "authentication unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		// The response has already started; the server middleware can record
		// the request, but a different HTTP status can no longer be sent.
		return
	}
}
