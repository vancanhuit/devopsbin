package httpapi

import (
	"context"
	"net/http"
)

// GetStatus implements the /status/{code} endpoint, responding with the HTTP
// status code supplied in the path. Codes outside the valid 100-599 range
// yield a 400 response.
func (s *Server) GetStatus(_ context.Context, request GetStatusRequestObject) (GetStatusResponseObject, error) {
	code := request.Code
	if code < 100 || code > 599 {
		return GetStatus400JSONResponse{Error: "status code must be in the range 100-599"}, nil
	}

	// 1xx informational, 204 No Content, and 304 Not Modified must not carry a
	// response body per RFC 9110, so emit only the status line for them.
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified {
		return statusNoBodyResponse(code), nil
	}

	var description *string
	if text := http.StatusText(int(code)); text != "" {
		description = &text
	}
	return GetStatusdefaultJSONResponse{
		Body:       StatusResponse{Code: code, Description: description},
		StatusCode: int(code),
	}, nil
}

// statusNoBodyResponse emits only the status line for status codes that must
// not carry a response body. It satisfies GetStatusResponseObject.
type statusNoBodyResponse int32

func (r statusNoBodyResponse) VisitGetStatusResponse(w http.ResponseWriter) error {
	w.WriteHeader(int(r))
	return nil
}
