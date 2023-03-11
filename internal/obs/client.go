// Package obs provides a basic obs-websocket 5.0 client. It supports all of
// the API calls needed for resetti to function.
//
// See https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md
// for more detailed documentation on the websocket protocol.
package obs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// Client manages an OBS websocket connection.
type Client struct {
	ws      *websocket.Conn
	mx      *sync.Mutex
	idCache idCache

	err map[uuid.UUID]chan error
	rcv map[uuid.UUID]chan json.RawMessage
}

// StringMap represents a JSON object.
type StringMap map[string]any

// batchResponse contains data sent back from OBS as a result of a request
// batch.
type batchResponse struct {
	Id      uuid.UUID         `json:"requestID"`
	Results []requestResponse `json:"results"`
}

// requestResponse contains data sent back from OBS as a result of a request.
type requestResponse struct {
	Id     uuid.UUID `json:"requestId"`
	Status struct {
		Result  bool   `json:"result"`
		Code    int    `json:"code"`
		Comment string `json:"comment"`
	} `json:"requestStatus"`
	Data json.RawMessage `json:"responseData,omitempty"`
}

// websocketResponse contains the data for a single JSON message received from OBS.
// It can represent any message received, such as an event or a request response.
type websocketResponse struct {
	Data json.RawMessage `json:"d"`
	Op   int             `json:"op"`
}

// Batch creates and *synchronously* submits a new request batch. The provided
// closure can be used to add requests to the batch. If the closure returns an
// error or panics, the batch will not be submitted.
func (c *Client) Batch(mode BatchMode, fn func(*Batch) error) (err error) {
	batch := newBatch(c)

	// Some of Batch's methods will panic when a scene item ID can not be
	// found to remove the need for tedious error handling code. Those
	// panics must be handled here.
	defer func() {
		result := recover()
		if res, ok := result.(error); ok {
			err = res
		} else if result != nil {
			err = errors.Errorf("%+v", result)
		}
	}()

	// Run the closure to fill the batch with requests.
	err = fn(&batch)
	if err != nil {
		return err
	}
	if len(batch.requests) == 0 {
		return errors.New("batch has no requests")
	}

	// Submit the batch.
	id := uuid.New()
	rawBatch := StringMap{
		"op": 8,
		"d": StringMap{
			"requestId":     id,
			"haltOnFailure": true,
			"executionType": mode,
			"requests":      batch.requests,
		},
	}
	errch := make(chan error, 1)
	c.mx.Lock()
	c.err[id] = errch
	c.mx.Unlock()
	err = wsjson.Write(context.Background(), c.ws, &rawBatch)
	if err != nil {
		c.mx.Lock()
		delete(c.err, id)
		c.mx.Unlock()
		return err
	}

	return <-errch
}

// Connect attempts to connect to an OBS instance at the given address. If
// authentication is required, the given password will be used.
func (c *Client) Connect(ctx context.Context, port uint16, pw string) (<-chan error, error) {
	// Setup websocket connection.
	c.mx = &sync.Mutex{}
	c.idCache = newIdCache()
	c.err = make(map[uuid.UUID]chan error)
	c.rcv = make(map[uuid.UUID]chan json.RawMessage)
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("wss://localhost:%d", port), nil)
	if err != nil {
		return nil, err
	}
	c.ws = conn

	// Respond to the hello message.
	if err = c.respondHello(ctx, pw); err != nil {
		c.ws.Close(websocket.StatusInternalError, "")
		return nil, err
	}

	// Wait for the Identified response.
	if err = c.identified(ctx); err != nil {
		c.ws.Close(websocket.StatusInternalError, "")
		return nil, err
	}

	// Start client loop.
	errch := make(chan error, 1)
	go c.run(ctx, errch)
	return errch, nil
}

func (c *Client) identified(ctx context.Context) error {
	type identifiedMessage struct {
		Data struct {
			RpcVersion int `json:"negotiatedRpcVersion"`
		} `json:"d"`
	}

	identified := identifiedMessage{}
	if err := wsjson.Read(ctx, c.ws, &identified); err != nil {
		return err
	}
	if identified.Data.RpcVersion != 1 {
		return errors.New("failed to negotiate rpc version")
	}
	return nil
}

func (c *Client) respondHello(ctx context.Context, pw string) error {
	type helloMessage struct {
		Data struct {
			WsVersion  string `json:"obsWebSocketVersion"`
			RpcVersion int    `json:"rpcVersion"`
			Auth       *struct {
				Challenge string `json:"challenge"`
				Salt      string `json:"salt"`
			} `json:"authentication,omitempty"`
		} `json:"d"`
	}

	hello := helloMessage{}
	if err := wsjson.Read(ctx, c.ws, &hello); err != nil {
		return err
	}
	if hello.Data.Auth == nil {
		// No authentication required.
		err := wsjson.Write(ctx, c.ws, StringMap{
			"op": 1,
			"d": StringMap{
				"rpcVersion": 1,
			},
		})
		if err != nil {
			return err
		}
	} else {
		// Authentication required.
		challenge := hello.Data.Auth.Challenge
		salt := hello.Data.Auth.Salt
		output := make([]byte, 0)
		sha := sha256.Sum256([]byte(pw + salt))
		base64.StdEncoding.Encode(output, sha[:])
		output = append(output, []byte(challenge)...)
		sha = sha256.Sum256(output)
		base64.StdEncoding.Encode(output, sha[:])
		err := wsjson.Write(ctx, c.ws, StringMap{
			"op": 1,
			"d": StringMap{
				"rpcVersion":     1,
				"authentication": string(output),
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) run(ctx context.Context, errch chan<- error) {
	defer c.ws.Close(websocket.StatusNormalClosure, "")
	for {
		// Check if the context has been cancelled.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read websocket message.
		res := websocketResponse{}
		if err := wsjson.Read(ctx, c.ws, &res); err != nil {
			errch <- errors.Wrap(err, "read json")
			return
		}

		switch res.Op {
		case 7:
			// Read the request response.
			data := requestResponse{}
			err := json.Unmarshal(res.Data, &data)
			if err != nil {
				errch <- errors.Wrap(err, "read request response")
				return
			}

			// Process the response.
			c.mx.Lock()
			if !data.Status.Result {
				c.err[data.Id] <- errors.Errorf("code %d: %s", data.Status.Code, data.Status.Comment)
			} else {
				c.rcv[data.Id] <- data.Data
			}
			delete(c.err, data.Id)
			delete(c.rcv, data.Id)
			c.mx.Unlock()
		case 9:
			// Request batch response.
			data := batchResponse{}
			err := json.Unmarshal(res.Data, &data)
			if err != nil {
				errch <- errors.Wrap(err, "read batch response")
				return
			}

			// Check for any errors.
			for _, result := range data.Results {
				if !result.Status.Result {
					err = errors.Errorf("code %d: %s", result.Status.Code, result.Status.Comment)
				}
			}

			// Send the result to whatever submitted the batch.
			c.mx.Lock()
			c.err[data.Id] <- err
			delete(c.err, data.Id)
			c.mx.Unlock()
		}
	}
}

func (c *Client) sendRequest(data request) (res json.RawMessage, err error) {
	// Prepare the request.
	id := uuid.New()
	data.Id = id
	resch := make(chan json.RawMessage, 1)
	errch := make(chan error, 1)
	req := StringMap{
		"op": 6,
		"d":  data,
	}

	// Send the request.
	c.mx.Lock()
	c.rcv[id] = resch
	c.err[id] = errch
	c.mx.Unlock()
	if err := wsjson.Write(context.Background(), c.ws, req); err != nil {
		c.mx.Lock()
		delete(c.rcv, id)
		delete(c.err, id)
		c.mx.Unlock()
		return nil, err
	}

	// Wait for a response.
	select {
	case res = <-resch:
	case err = <-errch:
	}
	return
}

// Requests

// getSceneItemId returns the ID of the given scene item if it exists.
func (c *Client) getSceneItemId(scene, name string) (int, error) {
	if id, ok := c.idCache.Get(scene, name); ok {
		return id, nil
	}
	req := reqGetSceneItemId(scene, name)
	data, err := c.sendRequest(req)
	if err != nil {
		return 0, err
	}
	res := struct {
		Id int `json:"sceneItemId"`
	}{}
	if err = json.Unmarshal(data, &res); err != nil {
		return 0, err
	}
	c.idCache.Set(scene, name, res.Id)
	return res.Id, nil
}

// AddSceneItem adds the source with the given name to the given scene.
func (c *Client) AddSceneItem(scene, name string) error {
	req := reqAddSceneItem(scene, name)
	_, err := c.sendRequest(req)
	return err
}

// CreateSceneCollection creates a new scene collection with the given name
// if one does not already exist.
func (c *Client) CreateSceneCollection(name string) error {
	req := reqCreateSceneCollection(name)
	_, err := c.sendRequest(req)
	return err
}

// CreateScene creates a new scene with the given name in the current scene
// collection.
func (c *Client) CreateScene(name string) error {
	req := reqCreateScene(name)
	_, err := c.sendRequest(req)
	return err
}

// CreateSource creates a new source with the given name and settings on the
// given scene.
func (c *Client) CreateSource(scene, name string, kind SourceKind, settings StringMap) error {
	req := reqCreateSource(scene, name, string(kind), settings)
	_, err := c.sendRequest(req)
	return err
}

// DeleteScene deletes the given scene.
func (c *Client) DeleteScene(name string) error {
	req := reqDeleteScene(name)
	_, err := c.sendRequest(req)
	return err
}

// GetCanvasSize returns the base resolution (canvas size) of the current OBS
// profile.
func (c *Client) GetCanvasSize() (width, height int, err error) {
	req := reqGetCanvasSize()
	data, err := c.sendRequest(req)
	if err != nil {
		return 0, 0, err
	}
	res := struct {
		Width  float64 `json:"baseWidth"`
		Height float64 `json:"baseHeight"`
	}{}
	err = json.Unmarshal(data, &res)
	return int(res.Width), int(res.Height), err
}

// GetSceneCollectionList returns a list of the existing scene collections and
// which one is currently active.
func (c *Client) GetSceneCollectionList() (collections []string, active string, err error) {
	req := reqGetSceneCollectionList()
	data, err := c.sendRequest(req)
	if err != nil {
		return nil, "", err
	}
	res := struct {
		Collections []string `json:"sceneCollections"`
		Active      string   `json:"currentSceneCollectionName"`
	}{}
	err = json.Unmarshal(data, &res)
	return res.Collections, res.Active, err
}

// GetSceneItemTransform returns the size and position of the given scene item.
func (c *Client) GetSceneItemTransform(scene, name string) (x, y, w, h float64, err error) {
	id, err := c.getSceneItemId(scene, name)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	req := reqGetSceneItemTransform(scene, id)
	data, err := c.sendRequest(req)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	res := struct {
		T struct {
			X      float64 `json:"positionX"`
			Y      float64 `json:"positionY"`
			Width  float64 `json:"boundsWidth"`
			Height float64 `json:"boundsHeight"`
		} `json:"sceneItemTransform"`
	}{}
	err = json.Unmarshal(data, &res)
	return res.T.X, res.T.Y, res.T.Width, res.T.Height, err
}

// GetSceneList returns a list of the existing scenes.
func (c *Client) GetSceneList() ([]string, error) {
	req := reqGetSceneList()
	data, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}
	res := struct {
		Scenes []struct {
			Name string `json:"sceneName"`
		} `json:"scenes"`
	}{}
	if err = json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	list := make([]string, 0)
	for _, scene := range res.Scenes {
		list = append(list, scene.Name)
	}
	return list, nil
}

// SetScene sets the current scene.
func (c *Client) SetScene(name string) error {
	req := reqSetScene(name)
	_, err := c.sendRequest(req)
	return err
}

// SetSceneCollection sets the current scene collection.
func (c *Client) SetSceneCollection(name string) error {
	req := reqSetSceneCollection(name)
	_, err := c.sendRequest(req)
	return err
}

// SetSceneItemBounds moves and resizes the given scene item.
func (c *Client) SetSceneItemBounds(scene, name string, x, y, w, h float64) error {
	id, err := c.getSceneItemId(scene, name)
	if err != nil {
		return err
	}
	req := reqSetSceneItemTransform(scene, id, x, y, w, h)
	_, err = c.sendRequest(req)
	return err
}

// SetSceneItemLocked locks or unlocks the given scene item.
func (c *Client) SetSceneItemLocked(scene, name string, locked bool) error {
	id, err := c.getSceneItemId(scene, name)
	if err != nil {
		return err
	}
	req := reqSetSceneItemLocked(scene, id, locked)
	_, err = c.sendRequest(req)
	return err
}

// SetSceneItemVisible hides or shows the given scene item.
func (c *Client) SetSceneItemVisible(scene, name string, visible bool) error {
	id, err := c.getSceneItemId(scene, name)
	if err != nil {
		return err
	}
	req := reqSetSceneItemVisible(scene, id, visible)
	_, err = c.sendRequest(req)
	return err
}

// SetSourceSettings configures the given source's settings.
func (c *Client) SetSourceSettings(name string, settings StringMap, overlay bool) error {
	req := reqSetSourceSettings(name, settings, overlay)
	_, err := c.sendRequest(req)
	return err
}
