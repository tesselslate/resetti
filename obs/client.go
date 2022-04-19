package obs

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/gorilla/websocket"
)

type OBSClient struct {
    c *websocket.Conn
}

func NewClient(port uint16, password string) (*OBSClient, error) {
    // setup websocket connection
    url := fmt.Sprintf("ws://localhost:%d", port)

    c, _, err := websocket.DefaultDialer.Dial(url, nil)
    if err != nil {
        c.Close()
        return nil, err
    }
    
    client := OBSClient{
        c,
    }

    // authenticate
    c.WriteJSON(GetAuthRequiredRequest())

    res := ResGetAuthRequired{}
    err = c.ReadJSON(&res)
    if err != nil {
        c.Close()
        return nil, err
    }

    if res.ID != "ok" {
        return nil, fmt.Errorf(res.Error)
    }

    // if we don't need to authenticate, we can return
    if !res.AuthRequired {
        return &client, nil
    }

    // otherwise, authenticate
    saltpwd := password + res.Salt
    salthash := sha256.Sum256([]byte(saltpwd))
    secret := base64.StdEncoding.EncodeToString(salthash[:])
    
    sec := secret + res.Challenge
    sechash := sha256.Sum256([]byte(sec))
    sec_res := base64.StdEncoding.EncodeToString(sechash[:])

    c.WriteJSON(AuthenticateRequest(sec_res))

    res_auth := ResAuthenticate{}
    err = c.ReadJSON(&res_auth)
    if err != nil {
        c.Close()
        return nil, err
    }

    if res_auth.ID != "ok" {
        return nil, fmt.Errorf(res_auth.Error)
    }

    return &client, nil
}
