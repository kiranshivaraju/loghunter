package response

import (
	"encoding/json"
	"net/http"
)

type envelope struct {
	Data any `json:"data"`
}

type collectionEnvelope struct {
	Data any            `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type PaginationMeta struct {
	Page    int  `json:"page"`
	Limit   int  `json:"limit"`
	Total   int  `json:"total"`
	HasNext bool `json:"has_next"`
}

func JSON(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Data: data})
}

func Created(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusCreated, envelope{Data: data})
}

func Accepted(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusAccepted, envelope{Data: data})
}

func Collection(w http.ResponseWriter, data any, meta PaginationMeta) {
	writeJSON(w, http.StatusOK, collectionEnvelope{Data: data, Meta: meta})
}

func Error(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
