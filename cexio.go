package cexio

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//API cex.io websocket API type
type API struct {
	//Key API key
	Key string
	//Secret API secret
	Secret string

	conn                *websocket.Conn
	responseSubscribers map[string]chan subscriberType
	subscriberMutex     sync.Mutex

	//Dialer used to connect to WebSocket server
	Dialer *websocket.Dialer
	//Logger used for error logging
	Logger *log.Logger
}

var apiURL = "wss://ws.cex.io/ws"

//NewAPI returns new API instance with default settings
func NewAPI(key string, secret string) *API {

	api := &API{
		Key:                 key,
		Secret:              secret,
		Dialer:              websocket.DefaultDialer,
		Logger:              log.New(os.Stderr, "", log.LstdFlags),
		responseSubscribers: map[string]chan subscriberType{},
		subscriberMutex:     sync.Mutex{},
	}

	return api
}

//Connect connects to cex.io websocket API server
func (a *API) Connect() error {

	sub := a.subscribe("connected")
	defer a.unsubscribe("connected")

	conn, _, err := a.Dialer.Dial(apiURL, nil)
	if err != nil {
		return err
	}
	a.conn = conn

	// run response from API server collector
	go a.responseCollector()

	<-sub //wait for connect response

	// run authentication
	err = a.auth()
	if err != nil {
		return err
	}

	return nil
}

//Close closes API connection
func (a *API) Close() error {
	err := a.conn.Close()
	if err != nil {
		return err
	}

	return nil
}

func (a *API) responseCollector() {
	defer a.Close()

	//todo: handle websocket connection close from server

	resp := responseAction{}

	for {
		_, msg, err := a.conn.ReadMessage()
		if err != nil {
			a.Logger.Println(err)
			return
		}

		err = json.Unmarshal(msg, &resp)
		if err != nil {
			a.Logger.Println(err)
		}

		sub, err := a.subscriber(resp.Action)
		if err != nil {
			a.Logger.Printf("No response handler for message: %s", string(msg))
			continue // don't know how to handle message so just skip it
		}

		sub <- msg
	}
}

func (a *API) auth() error {

	action := "auth"

	sub := a.subscribe(action)
	defer a.unsubscribe(action)

	timestamp := time.Now().Unix()

	// build signature string
	s := fmt.Sprintf("%d%s", timestamp, a.Key)

	h := hmac.New(sha256.New, []byte(a.Secret))
	h.Write([]byte(s))

	// generate signed signature string
	signature := hex.EncodeToString(h.Sum(nil))

	// build auth request
	request := requestAuthAction{
		E: action,
		Auth: requestAuthData{
			Key:       a.Key,
			Signature: signature,
			Timestamp: timestamp,
		},
	}

	// send auth request to API server
	err := a.conn.WriteJSON(request)
	if err != nil {
		return err
	}

	// wait for auth response from sever
	respMsg := <-sub

	resp := &responseAuth{}
	err = json.Unmarshal(respMsg, resp)
	if err != nil {
		return err
	}

	// check if authentication was successfull
	if resp.OK != "ok" || resp.Data.OK != "ok" {
		return errors.New(resp.Data.Error)
	}

	return nil
}

func (a *API) subscribe(action string) chan subscriberType {
	a.subscriberMutex.Lock()
	defer a.subscriberMutex.Unlock()

	a.responseSubscribers[action] = make(chan subscriberType)

	return a.responseSubscribers[action]
}

func (a *API) unsubscribe(action string) {
	a.subscriberMutex.Lock()
	defer a.subscriberMutex.Unlock()

	delete(a.responseSubscribers, action)
}

func (a *API) subscriber(action string) (chan subscriberType, error) {
	a.subscriberMutex.Lock()
	defer a.subscriberMutex.Unlock()

	sub, ok := a.responseSubscribers[action]
	if ok == false {
		return nil, fmt.Errorf("Subscriber '%s' not found", action)
	}

	return sub, nil
}
