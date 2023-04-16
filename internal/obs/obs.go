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
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// Client manages an OBS websocket connection.
type Client struct {
	ws      *websocket.Conn
	mu      sync.Mutex
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

// idCache is used to keep a cache of scene item IDs.
type idCache struct {
	mu    sync.RWMutex
	cache map[[2]string]int
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

// BatchAsync creates and submits a new request batch in another goroutine. If
// the batch errors, the error will be logged.
func (c *Client) BatchAsync(mode BatchMode, fn func(*Batch)) {
	batch, err := c.batchCreate(fn)
	if err != nil {
		log.Printf("BatchAsync creation failed: %s\n", err)
		return
	}
	go func() {
		if err := c.batchSubmit(mode, &batch); err != nil {
			log.Printf("BatchAsync submission failed: %s\n", err)
		}
	}()
}

// Batch creates and *synchronously* submits a new request batch. The provided
// closure can be used to add requests to the batch. If the closure returns an
// error or panics, the batch will not be submitted.
func (c *Client) Batch(mode BatchMode, fn func(*Batch)) error {
	batch, err := c.batchCreate(fn)
	if err != nil {
		return err
	}
	return c.batchSubmit(mode, &batch)
}

// Connect attempts to connect to an OBS instance at the given address. If
// authentication is required, the given password will be used.
func (c *Client) Connect(ctx context.Context, port uint16, pw string) (<-chan error, error) {
	// Setup websocket connection.
	c.idCache = idCache{sync.RWMutex{}, make(map[[2]string]int)}
	c.err = make(map[uuid.UUID]chan error)
	c.rcv = make(map[uuid.UUID]chan json.RawMessage)
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://localhost:%d", port), nil)
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

func (c *Client) batchCreate(fn func(*Batch)) (b Batch, err error) {
	batch := newBatch(c)

	// Some of Batch's methods will panic when a scene item ID can not be
	// found to remove the need for tedious error handling code. Those
	// panics must be handled here.
	defer func() {
		result := recover()
		if res, ok := result.(batchError); ok {
			err = res
		} else if result != nil {
			panic(result)
		}
	}()

	// Run the closure to fill the batch with requests.
	fn(&batch)
	if len(batch.requests) == 0 {
		return batch, errors.New("batch has no requests")
	}
	return batch, nil
}

func (c *Client) batchSubmit(mode BatchMode, batch *Batch) error {
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
	c.mu.Lock()
	c.err[id] = errch
	c.mu.Unlock()
	err := wsjson.Write(context.Background(), c.ws, &rawBatch)
	if err != nil {
		c.mu.Lock()
		delete(c.err, id)
		c.mu.Unlock()
		return err
	}

	return <-errch
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
	defer close(errch)
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
			errch <- fmt.Errorf("read json: %w", err)
			return
		}

		switch res.Op {
		case 7:
			// Read the request response.
			data := requestResponse{}
			err := json.Unmarshal(res.Data, &data)
			if err != nil {
				errch <- fmt.Errorf("read request response: %w", err)
				return
			}

			// Process the response.
			c.mu.Lock()
			if !data.Status.Result {
				c.err[data.Id] <- fmt.Errorf("code %d: %s", data.Status.Code, data.Status.Comment)
			} else {
				c.rcv[data.Id] <- data.Data
			}
			delete(c.err, data.Id)
			delete(c.rcv, data.Id)
			c.mu.Unlock()
		case 9:
			// Request batch response.
			data := batchResponse{}
			err := json.Unmarshal(res.Data, &data)
			if err != nil {
				errch <- fmt.Errorf("read batch response: %w", err)
				return
			}

			// Check for any errors.
			for _, result := range data.Results {
				if !result.Status.Result {
					err = fmt.Errorf("code %d: %s", result.Status.Code, result.Status.Comment)
				}
			}

			// Send the result to whatever submitted the batch.
			c.mu.Lock()
			c.err[data.Id] <- err
			delete(c.err, data.Id)
			c.mu.Unlock()
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
	c.mu.Lock()
	c.rcv[id] = resch
	c.err[id] = errch
	c.mu.Unlock()
	if err := wsjson.Write(context.Background(), c.ws, req); err != nil {
		c.mu.Lock()
		delete(c.rcv, id)
		delete(c.err, id)
		c.mu.Unlock()
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

// SetScene sets the current scene in a new goroutine and logs any errors
// that occur.
func (c *Client) SetScene(name string) {
	go func() {
		req := reqSetScene(name)
		if _, err := c.sendRequest(req); err != nil {
			log.Printf("SetScene error: %s\n", err)
		}
	}()
}

// SetSceneItemBounds moves and resizes the given scene item in a new
// goroutine and logs any errors that occur.
func (c *Client) SetSceneItemBounds(scene, name string, x, y, w, h float64) {
	go func() {
		id, err := c.getSceneItemId(scene, name)
		if err != nil {
			log.Printf("SetSceneItemBounds error: %s\n", err)
		}
		req := reqSetSceneItemTransform(scene, id, x, y, w, h)
		if _, err = c.sendRequest(req); err != nil {
			log.Printf("SetSceneItemBounds error: %s\n", err)
		}
	}()
}

// SetSceneItemVisible hides or shows the given scene item in a new
// goroutine and logs any errors that occur.
func (c *Client) SetSceneItemVisible(scene, name string, visible bool) {
	go func() {
		id, err := c.getSceneItemId(scene, name)
		if err != nil {
			log.Printf("SetSceneItemVisible error: %s\n", err)
		}
		req := reqSetSceneItemVisible(scene, id, visible)
		if _, err = c.sendRequest(req); err != nil {
			log.Printf("SetSceneItemVisible error: %s\n", err)
		}
	}()
}

// SetSourceFilterEnabled enables or disables a given filter in a new goroutine
// and logs any errors that occur.
func (c *Client) SetSourceFilterEnabled(source, filter string, enabled bool) {
	go func() {
		req := reqSetSourceFilterEnabled(source, filter, enabled)
		_, err := c.sendRequest(req)
		if err != nil {
			log.Printf("SetSourceFilterEnabled error: %s\n", err)
		}
	}()
}

// Get returns the ID of the given scene/source pair if it exists.
func (i *idCache) Get(scene string, name string) (int, bool) {
	i.mu.RLock()
	id, ok := i.cache[[2]string{scene, name}]
	i.mu.RUnlock()
	return id, ok
}

// Set inserts the given scene/source pair into the cache.
func (i *idCache) Set(scene string, name string, id int) {
	i.mu.Lock()
	i.cache[[2]string{scene, name}] = id
	i.mu.Unlock()
}
