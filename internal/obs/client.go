// Package obs provides a basic obs-websocket 5.0 client. It supports all of
// the API calls needed for resetti to function.
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
	ctx     context.Context
	ws      *websocket.Conn
	mx      *sync.Mutex
	idCache idCache

	err map[uuid.UUID]chan error
	rcv map[uuid.UUID]chan json.RawMessage
}

type StringMap map[string]any

// websocketResponse contains the data for a single JSON message received from OBS.
// It can represent any message received, such as an event or a request response.
type websocketResponse struct {
	Data json.RawMessage `json:"d"`
	Op   int             `json:"op"`
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

// Connect attempts to connect to an OBS instance at the given address. If
// authentication is required, the given password will be used.
func (c *Client) Connect(ctx context.Context, addr string, pw string) (<-chan error, error) {
	// Setup websocket connection.
	c.ctx = ctx
	c.mx = &sync.Mutex{}
	c.idCache = newIdCache()
	c.err = make(map[uuid.UUID]chan error)
	c.rcv = make(map[uuid.UUID]chan json.RawMessage)
	conn, _, err := websocket.Dial(ctx, "ws://"+addr, nil)
	if err != nil {
		return nil, err
	}
	c.ws = conn

	// Respond to the hello message.
	respondHello := func() error {
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
		if err = wsjson.Read(ctx, c.ws, &hello); err != nil {
			return err
		}
		if hello.Data.Auth == nil {
			// No authentication required.
			err = wsjson.Write(ctx, c.ws, StringMap{
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
			err = wsjson.Write(ctx, c.ws, StringMap{
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
	if err = respondHello(); err != nil {
		c.ws.Close(websocket.StatusInternalError, "")
		return nil, err
	}

	// Wait for the Identified response.
	identified := func() error {
		type identifiedMessage struct {
			Data struct {
				RpcVersion int `json:"negotiatedRpcVersion"`
			} `json:"d"`
		}

		identified := identifiedMessage{}
		if err = wsjson.Read(ctx, c.ws, &identified); err != nil {
			return err
		}
		if identified.Data.RpcVersion != 1 {
			return errors.New("failed to negotiate rpc version")
		}
		return nil
	}
	if err = identified(); err != nil {
		c.ws.Close(websocket.StatusInternalError, "")
		return nil, err
	}

	// Start client loop.
	errch := make(chan error, 16)
	go c.run(ctx, errch)
	return errch, nil
}

// request sends a request to the obs-websocket server with the given type and data.
func (c *Client) request(data any, name string) (json.RawMessage, error) {
	type rawRequest struct {
		Op   int       `json:"op"`
		Data StringMap `json:"d"`
	}

	id := uuid.New()
	errch := make(chan error, 1)
	resch := make(chan json.RawMessage, 1)
	c.mx.Lock()
	c.err[id] = errch
	c.rcv[id] = resch
	c.mx.Unlock()
	err := wsjson.Write(c.ctx, c.ws, rawRequest{
		Op: 6,
		Data: StringMap{
			"requestType": name,
			"requestId":   id,
			"requestData": data,
		},
	})
	if err != nil {
		c.mx.Lock()
		delete(c.err, id)
		delete(c.rcv, id)
		c.mx.Unlock()
		return nil, err
	}
	select {
	case err := <-errch:
		return nil, err
	case res := <-resch:
		return res, nil
	}
}

// run starts the main client loop. It listens for messages from the
// obs-websocket server until an error occurs with the websocket or the
// provided context is cancelled.
func (c *Client) run(ctx context.Context, errch chan<- error) {
	go func() {
		defer c.ws.Close(websocket.StatusNormalClosure, "")
		for {
			// Check if the context has been cancelled.
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read the next message.
			res := websocketResponse{}
			if err := wsjson.Read(ctx, c.ws, &res); err != nil {
				errch <- err
				return
			}

			// Discard anything that isn't a request response.
			if res.Op != 7 {
				continue
			}
			data := requestResponse{}
			err := json.Unmarshal(res.Data, &data)
			if err != nil {
				errch <- err
				continue
			}
			c.mx.Lock()
			if !data.Status.Result {
				c.err[data.Id] <- fmt.Errorf("code %d: %s", data.Status.Code, data.Status.Comment)
			} else {
				c.rcv[data.Id] <- data.Data
			}
			delete(c.err, data.Id)
			delete(c.rcv, data.Id)
			c.mx.Unlock()
		}
	}()
}
