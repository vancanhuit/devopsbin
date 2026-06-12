package httpapi

import (
	"context"
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

// GetEcho implements the /echo endpoint, reflecting the incoming request's
// method, path, query parameters, headers, and origin IP.
func (s *Server) GetEcho(ctx context.Context, _ GetEchoRequestObject) (GetEchoResponseObject, error) {
	resp := GetEcho200JSONResponse{Headers: HeaderMap{}, Query: HeaderMap{}}
	if r := requestFrom(ctx); r != nil {
		resp.Method = r.Method
		resp.Path = r.URL.Path
		resp.Query = HeaderMap(r.URL.Query())
		resp.Headers = headerMap(r)
		resp.Origin = originIP(r)
	}
	return resp, nil
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

// originIP returns the caller's IP address. The ClientIPFromRemoteAddr
// middleware normalizes RemoteAddr to a bare IP; fall back to splitting a
// host:port form defensively.
func originIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
