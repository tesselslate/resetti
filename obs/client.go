// Package obs provides a basic OBS websocket client implementation.
package obs

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// TODO: Properly handle OBS websocket responses. This might be worthwhile
// to split into a separate Go project entirely, since current OBS websocket
// libraries mostly appear unmaintained.

// Client maintains an OBS websocket connection.
type Client struct {
	active bool
	conn   *websocket.Conn
	mx     sync.Mutex
}

// NewClient creates a new OBSClient and connects to the OBS websocket server.
func NewClient(port uint16, password string) (*Client, error) {
	// Setup websocket connection.
	url := fmt.Sprintf("ws://localhost:%d", port)

	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		c.Close()
		return nil, err
	}

	client := Client{
		active: true,
		conn:   c,
		mx:     sync.Mutex{},
	}

	// Authenticate with OBS, if needed.
	c.WriteJSON(GetAuthRequiredRequest())

	res := ResGetAuthRequired{}
	if err = c.ReadJSON(&res); err != nil {
		c.Close()
		return nil, err
	}

	if res.Status != "ok" {
		return nil, fmt.Errorf(res.Error)
	}

	// If we don't need to authenticate, we can return early.
	if !res.AuthRequired {
		return &client, nil
	}

	// Otherwise, we authenticate. Refer to the OBS websocket protocol
	// documentation for more information on how the auth flow works.
	saltpwd := password + res.Salt
	salthash := sha256.Sum256([]byte(saltpwd))
	secret := base64.StdEncoding.EncodeToString(salthash[:])

	sec := secret + res.Challenge
	sechash := sha256.Sum256([]byte(sec))
	secRes := base64.StdEncoding.EncodeToString(sechash[:])

	c.WriteJSON(AuthenticateRequest(secRes))

	resAuth := ResAuthenticate{}
	if err = c.ReadJSON(&resAuth); err != nil {
		c.Close()
		return nil, err
	}

	if resAuth.Status != "ok" {
		return nil, fmt.Errorf(resAuth.Error)
	}

	return &client, nil
}

// SetCurrentScene sets the current scene being recorded in OBS.
func (c *Client) SetCurrentScene(scene string) error {
	c.mx.Lock()
	defer c.mx.Unlock()

	return c.conn.WriteJSON(SetCurrentSceneRequest(scene))
}
