package obs

import "github.com/google/uuid"

func SetCurrentSceneRequest(scene string) ReqSetCurrentScene {
	return ReqSetCurrentScene{
		ID:   uuid.New().String(),
		Type: "SetCurrentScene",
		Name: scene,
	}
}

type ReqSetCurrentScene struct {
	// boilerplate fields
	ID   string `json:"message-id"`
	Type string `json:"request-type"`

	// request fields
	Name string `json:"scene-name"`
}

type ResSetCurrentScene struct {
	// boilerplate fields
	ID     string `json:"message-id"`
	Status string `json:"status"`
	Error  string `json:"error"`

	// response fields
}
