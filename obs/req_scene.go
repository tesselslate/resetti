package obs

import "github.com/google/uuid"

// SetCurrentSceneRequest creates a new SetCurrentScene request.
func SetCurrentSceneRequest(scene string) ReqSetCurrentScene {
	return ReqSetCurrentScene{
		ID:   uuid.New().String(),
		Type: "SetCurrentScene",
		Name: scene,
	}
}

// ReqSetCurrentScene represents a server-bound request to set
// the current OBS scene.
type ReqSetCurrentScene struct {
	ID   string `json:"message-id"`
	Type string `json:"request-type"`
	Name string `json:"scene-name"`
}

// ResSetCurrentScene represents a client-bound response to
// tell if the SetCurrentScene request was successful.
type ResSetCurrentScene struct {
	ID     string `json:"message-id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}
