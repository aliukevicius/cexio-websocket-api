package cexio

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

//API cex.io websocket API type
type API struct {
	//Key API key
	Key string
	//Secret API secret
	Secret string

	conn *websocket.Conn

	//Dialer used to connect to WebSocket server
	Dialer *websocket.Dialer
	//Logger used for error logging
	Logger *log.Logger

	//Messages channel which is used for reading responses from API
	Messages chan response
}

var apiURL = "wss://ws.cex.io/ws"

//NewAPI returns new API instance with default settings
func NewAPI(key string, secret string) *API {

	api := &API{
		Key:      key,
		Secret:   secret,
		Dialer:   websocket.DefaultDialer,
		Logger:   log.New(os.Stderr, "", log.LstdFlags),
		Messages: make(chan response),
	}

	return api
}

//Connect connects to cex.io websocket API server
func (a *API) Connect() error {

	conn, _, err := a.Dialer.Dial(apiURL, nil)
	if err != nil {
		return err
	}

	a.conn = conn

	go a.reader()

	return nil
}

//Close closes API connection
func (a *API) Close() error {
	err := a.conn.Close()
	if err != nil {
		return err
	}

	close(a.Messages)

	return nil
}

func (a *API) reader() {
	defer a.Close()

	for {
		_, msg, err := a.conn.ReadMessage()
		if err != nil {
			a.Logger.Println(err)
			return
		}

		r := response{}

		err = json.Unmarshal(msg, &r)

		a.Messages <- r
	}
}

func (a *API) Auth() error {

	timestamp := time.Now().Unix()

	s := fmt.Sprintf("%d%s", timestamp, a.Key)

	h := hmac.New(sha256.New, []byte(a.Secret))
	h.Write([]byte(s))

	signature := hex.EncodeToString(h.Sum(nil))

	request := requestAuthAction{
		E: "auth",
		Auth: requestAuthData{
			Key:       a.Key,
			Signature: signature,
			Timestamp: timestamp,
		},
	}

	// send auth message to API server
	err := a.conn.WriteJSON(request)
	if err != nil {
		return err
	}

	return nil
}
