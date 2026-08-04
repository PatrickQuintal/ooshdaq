package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (a *API) writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		a.logger.Error("encode JSON response", "status", status)
	}
}

func (a *API) writeError(response http.ResponseWriter, status int, code, message string) {
	a.writeJSON(response, status, errorEnvelope{Error: apiError{Code: code, Message: message}})
}

func (a *API) writeFieldError(response http.ResponseWriter, status int, code, message, field string) {
	a.writeJSON(response, status, errorEnvelope{
		Error: apiError{Code: code, Message: message, Field: field},
	})
}

func (a *API) decodeJSON(response http.ResponseWriter, request *http.Request, destination any) *apiError {
	contentType := request.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return &apiError{
			Code:    "unsupported_media_type",
			Message: "Content-Type must be application/json.",
		}
	}

	request.Body = http.MaxBytesReader(response, request.Body, a.maxBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &apiError{Code: "request_too_large", Message: "The request body is too large."}
		}
		return &apiError{Code: "invalid_request", Message: "The request body must contain one valid JSON object."}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &apiError{Code: "request_too_large", Message: "The request body is too large."}
		}
		return &apiError{Code: "invalid_request", Message: "The request body must contain one valid JSON object."}
	}
	return nil
}

func (a *API) requireEmptyBody(response http.ResponseWriter, request *http.Request) *apiError {
	request.Body = http.MaxBytesReader(response, request.Body, 1)
	var one [1]byte
	count, err := request.Body.Read(one[:])
	if count != 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return &apiError{Code: "invalid_request", Message: "This request does not accept a body."}
	}
	return nil
}

func (a *API) writeRequestError(response http.ResponseWriter, apiErr *apiError) {
	status := http.StatusBadRequest
	switch apiErr.Code {
	case "unsupported_media_type":
		status = http.StatusUnsupportedMediaType
	case "request_too_large":
		status = http.StatusRequestEntityTooLarge
	}
	if apiErr.Field == "" {
		a.writeError(response, status, apiErr.Code, apiErr.Message)
		return
	}
	a.writeFieldError(response, status, apiErr.Code, apiErr.Message, apiErr.Field)
}
