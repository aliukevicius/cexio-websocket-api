package cexio

import (
	"log"
	"os"

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
	Messages chan []byte
}

var apiURL = "wss://ws.cex.io/ws"

//NewAPI returns new API instance with default settings
func NewAPI(key string, secret string) *API {

	api := &API{
		Key:      key,
		Secret:   secret,
		Dialer:   websocket.DefaultDialer,
		Logger:   log.New(os.Stderr, "", log.LstdFlags),
		Messages: make(chan []byte),
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

		a.Messages <- msg
	}
}
