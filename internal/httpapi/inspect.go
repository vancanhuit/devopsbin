package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/google/uuid"
)

// GetUuid implements the /uuid endpoint, returning a random version 4 UUID.
func (s *Server) GetUuid(_ context.Context, _ GetUuidRequestObject) (GetUuidResponseObject, error) {
	return GetUuid200JSONResponse{Uuid: uuid.New()}, nil
}

// GetIp implements the /ip endpoint, returning the caller's origin IP address.
func (s *Server) GetIp(ctx context.Context, _ GetIpRequestObject) (GetIpResponseObject, error) {
	return GetIp200JSONResponse{Origin: originIP(requestFrom(ctx))}, nil
}

// GetHeaders implements the /headers endpoint, echoing the request headers.
func (s *Server) GetHeaders(ctx context.Context, _ GetHeadersRequestObject) (GetHeadersResponseObject, error) {
	return GetHeaders200JSONResponse{Headers: headerMap(requestFrom(ctx))}, nil
}

// GetUserAgent implements the /user-agent endpoint, echoing the User-Agent
// header.
func (s *Server) GetUserAgent(ctx context.Context, _ GetUserAgentRequestObject) (GetUserAgentResponseObject, error) {
	var ua string
	if r := requestFrom(ctx); r != nil {
		ua = r.UserAgent()
	}
	return GetUserAgent200JSONResponse{UserAgent: ua}, nil
}

// GetScheme implements the /scheme endpoint, returning the request scheme
// (http or https).
func (s *Server) GetScheme(ctx context.Context, _ GetSchemeRequestObject) (GetSchemeResponseObject, error) {
	return GetScheme200JSONResponse{Scheme: SchemeResponseScheme(requestScheme(requestFrom(ctx)))}, nil
}

// GetEcho implements the /echo endpoint, reflecting the incoming request's
// method, path, query parameters, headers, origin IP, and scheme.
func (s *Server) GetEcho(ctx context.Context, _ GetEchoRequestObject) (GetEchoResponseObject, error) {
	return GetEcho200JSONResponse(echoResponse(ctx, nil)), nil
}

// PostEcho implements POST /echo, reflecting the incoming request along with
// its body.
func (s *Server) PostEcho(ctx context.Context, request PostEchoRequestObject) (PostEchoResponseObject, error) {
	resp, ok := echoWithBody(ctx, request.Body)
	if !ok {
		return PostEcho413JSONResponse{EchoBodyTooLargeJSONResponse{Error: echoBodyTooLargeMessage}}, nil
	}
	return PostEcho200JSONResponse{EchoReflectionJSONResponse(resp)}, nil
}

// PutEcho implements PUT /echo, reflecting the incoming request along with its
// body.
func (s *Server) PutEcho(ctx context.Context, request PutEchoRequestObject) (PutEchoResponseObject, error) {
	resp, ok := echoWithBody(ctx, request.Body)
	if !ok {
		return PutEcho413JSONResponse{EchoBodyTooLargeJSONResponse{Error: echoBodyTooLargeMessage}}, nil
	}
	return PutEcho200JSONResponse{EchoReflectionJSONResponse(resp)}, nil
}

// PatchEcho implements PATCH /echo, reflecting the incoming request along with
// its body.
func (s *Server) PatchEcho(ctx context.Context, request PatchEchoRequestObject) (PatchEchoResponseObject, error) {
	resp, ok := echoWithBody(ctx, request.Body)
	if !ok {
		return PatchEcho413JSONResponse{EchoBodyTooLargeJSONResponse{Error: echoBodyTooLargeMessage}}, nil
	}
	return PatchEcho200JSONResponse{EchoReflectionJSONResponse(resp)}, nil
}

// DeleteEcho implements DELETE /echo, reflecting the incoming request along
// with its body.
func (s *Server) DeleteEcho(ctx context.Context, request DeleteEchoRequestObject) (DeleteEchoResponseObject, error) {
	resp, ok := echoWithBody(ctx, request.Body)
	if !ok {
		return DeleteEcho413JSONResponse{EchoBodyTooLargeJSONResponse{Error: echoBodyTooLargeMessage}}, nil
	}
	return DeleteEcho200JSONResponse{EchoReflectionJSONResponse(resp)}, nil
}

// maxEchoBodyBytes caps how large a request body the /echo endpoint will
// reflect back. Larger bodies yield a 413 response.
const maxEchoBodyBytes = 64 << 10 // 64 KiB

// echoBodyTooLargeMessage is the error returned when a request body exceeds
// maxEchoBodyBytes.
var echoBodyTooLargeMessage = fmt.Sprintf(
	"request body exceeds the maximum echo size of %d bytes", maxEchoBodyBytes,
)

// echoResponse builds the reflection of the request stored in ctx. The optional
// body is echoed back verbatim; a nil body leaves the response's body field
// unset. A nil request (no value in ctx) yields empty, non-nil maps so the JSON
// response is always an object.
func echoResponse(ctx context.Context, body *string) EchoResponse {
	resp := EchoResponse{
		Headers: HeaderMap{},
		Query:   HeaderMap{},
		Scheme:  EchoResponseSchemeHttp,
		Body:    body,
	}
	if r := requestFrom(ctx); r != nil {
		resp.Method = r.Method
		resp.Path = r.URL.Path
		resp.Query = HeaderMap(r.URL.Query())
		resp.Headers = headerMap(r)
		resp.Origin = originIP(r)
		resp.Scheme = EchoResponseScheme(requestScheme(r))
	}
	return resp
}

// echoWithBody reflects the request stored in ctx together with the given body.
// It reports false (and an empty response) when the body exceeds
// maxEchoBodyBytes, signaling the caller to return a 413.
func echoWithBody(ctx context.Context, body *string) (EchoResponse, bool) {
	if body != nil && len(*body) > maxEchoBodyBytes {
		return EchoResponse{}, false
	}
	return echoResponse(ctx, body), true
}

// headerMap converts the request headers into the generated HeaderMap shape. A
// nil request yields an empty (non-nil) map so the JSON response is always an
// object.
func headerMap(r *http.Request) HeaderMap {
	if r == nil {
		return HeaderMap{}
	}
	out := make(HeaderMap, len(r.Header))
	for name, values := range r.Header {
		out[name] = values
	}
	return out
}

// originIP returns the caller's IP address. When a trusted proxy resolved the
// real client IP (see trustedProxy), that value is used; otherwise it falls
// back to the connecting peer's RemoteAddr, splitting a host:port form
// defensively.
func originIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if ip, ok := clientIPFrom(r.Context()); ok {
		return ip.String()
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// requestScheme returns the scheme ("http" or "https") of the request. When a
// trusted proxy resolved the forwarded scheme (see trustedProxy), that value
// is used; otherwise it reflects whether this server terminated TLS for the
// connection. A nil request defaults to "http".
func requestScheme(r *http.Request) string {
	if r == nil {
		return "http"
	}
	if scheme, ok := schemeFrom(r.Context()); ok {
		return scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
