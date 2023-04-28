package obs

import "github.com/google/uuid"

// Source kinds.
const (
	KindWindow SourceKind = "xcomposite_input"
	KindImage  SourceKind = "image_source"
)

// SourceKind contains the input type for a given source.
type SourceKind string

type request struct {
	Type string    `json:"requestType"`
	Id   uuid.UUID `json:"requestId"`
	Data StringMap `json:"requestData"`
}

func reqGetCanvasSize() request {
	return request{Type: "GetVideoSettings"}
}

func reqGetSceneItemId(scene, name string) request {
	return request{
		Type: "GetSceneItemId",
		Data: StringMap{
			"sceneName":  scene,
			"sourceName": name,
		},
	}
}

func reqGetSceneItemIndex(scene string, id int) request {
	return request{
		Type: "GetSceneItemIndex",
		Data: StringMap{
			"sceneName":   scene,
			"sceneItemId": id,
		},
	}
}

func reqGetSceneItemTransform(scene string, id int) request {
	return request{
		Type: "GetSceneItemTransform",
		Data: StringMap{
			"sceneName":   scene,
			"sceneItemId": id,
		},
	}
}

func reqSetScene(scene string) request {
	return request{
		Type: "SetCurrentProgramScene",
		Data: StringMap{
			"sceneName": scene,
		},
	}
}

func reqSetSceneItemIndex(scene string, id int, index int) request {
	return request{
		Type: "SetSceneItemIndex",
		Data: StringMap{
			"sceneName":      scene,
			"sceneItemId":    id,
			"sceneItemIndex": index,
		},
	}
}

func reqSetSceneItemTransform(scene string, id int, x, y, w, h float64) request {
	return request{
		Type: "SetSceneItemTransform",
		Data: StringMap{
			"sceneName":   scene,
			"sceneItemId": id,
			"sceneItemTransform": StringMap{
				"positionX":    x,
				"positionY":    y,
				"boundsWidth":  w,
				"boundsHeight": h,
				"boundsType":   "OBS_BOUNDS_STRETCH", // We never use any other stretch type.
			},
		},
	}
}

func reqSetSceneItemVisible(scene string, id int, visible bool) request {
	return request{
		Type: "SetSceneItemEnabled",
		Data: StringMap{
			"sceneName":        scene,
			"sceneItemId":      id,
			"sceneItemEnabled": visible,
		},
	}
}

func reqSetSourceFilterEnabled(source, filter string, enabled bool) request {
	return request{
		Type: "SetSourceFilterEnabled",
		Data: StringMap{
			"sourceName":    source,
			"filterName":    filter,
			"filterEnabled": enabled,
		},
	}
}

func reqSetSourceSettings(name string, settings StringMap, keep bool) request {
	return request{
		Type: "SetInputSettings",
		Data: StringMap{
			"inputName":     name,
			"inputSettings": settings,
			"overlay":       keep,
		},
	}
}
