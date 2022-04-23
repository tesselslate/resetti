package obs

import "github.com/google/uuid"

// GetAuthRequiredRequest creates a new GetAuthRequired request.
func GetAuthRequiredRequest() ReqGetAuthRequired {
	return ReqGetAuthRequired{
		ID:   uuid.New().String(),
		Type: "GetAuthRequired",
	}
}

// ReqGetAuthRequired represents a server-bound request
// to check if authentication is needed.
type ReqGetAuthRequired struct {
	ID   string `json:"message-id"`
	Type string `json:"request-type"`
}

// ResGetAuthRequired represents a client-bound response
// telling if authentication is needed.
type ResGetAuthRequired struct {
	ID           string `json:"message-id"`
	Status       string `json:"status"`
	Error        string `json:"error"`
	AuthRequired bool   `json:"authRequired"`
	Challenge    string `json:"challenge"`
	Salt         string `json:"salt"`
}

// AuthenticateRequest creates a new Authenticate request.
func AuthenticateRequest(auth string) ReqAuthenticate {
	return ReqAuthenticate{
		ID:   uuid.New().String(),
		Type: "Authenticate",
		Auth: auth,
	}
}

// ReqAuthenticate represents a server-bound request to
// authenticate.
type ReqAuthenticate struct {
	ID   string `json:"message-id"`
	Type string `json:"request-type"`
	Auth string `json:"auth"`
}

// ResAuthenticate represents a client-bound response telling
// if the authentication was successful.
type ResAuthenticate struct {
	ID     string `json:"message-id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}
