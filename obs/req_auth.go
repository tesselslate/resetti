package obs

import "github.com/google/uuid"

func GetAuthRequiredRequest() ReqGetAuthRequired {
	return ReqGetAuthRequired{
		ID:   uuid.New().String(),
		Type: "GetAuthRequired",
	}
}

type ReqGetAuthRequired struct {
	// boilerplate fields
	ID   string `json:"message-id"`
	Type string `json:"request-type"`

	// request fields
}

type ResGetAuthRequired struct {
	// boilerplate fields
	ID     string `json:"message-id"`
	Status string `json:"status"`
	Error  string `json:"error"`

	// response fields
	AuthRequired bool   `json:"authRequired"`
	Challenge    string `json:"challenge"`
	Salt         string `json:"salt"`
}

func AuthenticateRequest(auth string) ReqAuthenticate {
	return ReqAuthenticate{
		ID:   uuid.New().String(),
		Type: "Authenticate",
		Auth: auth,
	}
}

type ReqAuthenticate struct {
	// boilerplate fields
	ID   string `json:"message-id"`
	Type string `json:"request-type"`

	// request fields
	Auth string `json:"auth"`
}

type ResAuthenticate struct {
	// boilerplate fields
	ID     string `json:"message-id"`
	Status string `json:"status"`
	Error  string `json:"error"`

	// response fields
}
