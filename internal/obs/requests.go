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

func reqAddSceneItem(scene, name string) request {
	return request{
		Type: "CreateSceneItem",
		Data: StringMap{
			"sceneName":  scene,
			"sourceName": name,
		},
	}
}

func reqCreateSceneCollection(name string) request {
	return request{
		Type: "CreateSceneCollection",
		Data: StringMap{
			"sceneCollectionName": name,
		},
	}
}

func reqCreateScene(name string) request {
	return request{
		Type: "CreateScene",
		Data: StringMap{
			"sceneName": name,
		},
	}
}

func reqCreateSource(scene, name, kind string, settings StringMap) request {
	return request{
		Type: "CreateInput",
		Data: StringMap{
			"sceneName": scene,
			"inputName": name,
			"inputKind": kind,
			"settings":  settings,
		},
	}
}

func reqDeleteScene(name string) request {
	return request{
		Type: "RemoveScene",
		Data: StringMap{
			"sceneName": name,
		},
	}
}

func reqGetCanvasSize() request {
	return request{Type: "GetVideoSettings"}
}

func reqGetSceneCollectionList() request {
	return request{Type: "GetSceneCollectionList"}
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

func reqGetSceneItemTransform(scene string, id int) request {
	return request{
		Type: "GetSceneItemTransform",
		Data: StringMap{
			"sceneName":   scene,
			"sceneItemId": id,
		},
	}
}

func reqGetSceneList() request {
	return request{Type: "GetSceneList"}
}

func reqSetScene(scene string) request {
	return request{
		Type: "SetCurrentProgramScene",
		Data: StringMap{
			"sceneName": scene,
		},
	}
}

func reqSetSceneCollection(collection string) request {
	return request{
		Type: "SetCurrentSceneCollection",
		Data: StringMap{
			"sceneCollectionName": collection,
		},
	}
}

func reqSetSceneItemLocked(scene string, id int, locked bool) request {
	return request{
		Type: "SetSceneItemLocked",
		Data: StringMap{
			"sceneName":       scene,
			"sceneItemId":     id,
			"sceneItemLocked": locked,
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
