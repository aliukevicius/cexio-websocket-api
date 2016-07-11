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
	orderBookHandlers   map[string]chan bool

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
		orderBookHandlers:   map[string]chan bool{},
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

//Ticker send ticker request
func (a *API) Ticker(cCode1 string, cCode2 string) (*responseTicker, error) {

	action := "ticker"

	sub := a.subscribe(action)
	defer a.unsubscribe(action)

	timestamp := time.Now().UnixNano()

	msg := requestTicker{
		E:    action,
		Data: []string{cCode1, cCode2},
		Oid:  fmt.Sprintf("%d_%s:%s", timestamp, cCode1, cCode2),
	}

	err := a.conn.WriteJSON(msg)
	if err != nil {
		return nil, err
	}

	// wait for response from sever
	respMsg := (<-sub).([]byte)

	resp := &responseTicker{}
	err = json.Unmarshal(respMsg, resp)
	if err != nil {
		return nil, err
	}

	// check if authentication was successfull
	if resp.OK != "ok" {
		return nil, errors.New(resp.Data.Error)
	}

	return resp, nil
}

//OrderBookSubscribe subscribes to order book updates.
//Order book snapshot will come as a first update
func (a *API) OrderBookSubscribe(cCode1 string, cCode2 string, depth int64, handler subscriptionHandler) error {

	action := "order-book-subscribe"

	subscriptionIdentifier := fmt.Sprintf("%s_%s:%s", action, cCode1, cCode2)

	a.subscribe(subscriptionIdentifier)

	timestamp := time.Now().UnixNano()

	req := requestOrderBookSubscribe{
		E:   action,
		Oid: fmt.Sprintf("%d_%s:%s", timestamp, cCode1, cCode2),
		Data: requestOrderBookSubscribeData{
			Pair:      []string{cCode1, cCode2},
			Subscribe: true,
			Depth:     depth,
		},
	}

	err := a.conn.WriteJSON(req)
	if err != nil {
		return err
	}

	go a.handleOrderBookSubscriptions(subscriptionIdentifier, handler)

	return nil
}

func (a *API) handleOrderBookSubscriptions(subscriptionIdentifier string, handler subscriptionHandler) {

	quit := make(chan bool)

	a.orderBookHandlers[subscriptionIdentifier] = quit

	sub, err := a.subscriber(subscriptionIdentifier)
	if err != nil {
		a.Logger.Println(err)
		return
	}

	for {
		select {
		case <-quit:
			return
		case m := <-sub:

			obData := OrderBookUpdateData{}

			if resp, ok := m.(*responseOrderBookSubscribe); ok {

				obData = OrderBookUpdateData{
					ID:        resp.Data.ID,
					Pair:      resp.Data.Pair,
					Timestamp: resp.Data.Timestamp,
					Bids:      resp.Data.Bids,
					Asks:      resp.Data.Asks,
				}
			}

			if resp, ok := m.(*responseOrderBookUpdate); ok {

				obData = OrderBookUpdateData{
					ID:        resp.Data.ID,
					Pair:      resp.Data.Pair,
					Timestamp: resp.Data.Timestamp,
					Bids:      resp.Data.Bids,
					Asks:      resp.Data.Asks,
				}
			}

			handler(obData)
		}
	}
}

//OrderBookUnsubscribe unsubscribes from order book updates
func (a *API) OrderBookUnsubscribe(cCode1 string, cCode2 string) error {

	action := "order-book-unsubscribe"

	sub := a.subscribe(action)
	defer a.unsubscribe(action)

	timestamp := time.Now().UnixNano()

	req := requestOrderBookUnsubscribe{
		E:   action,
		Oid: fmt.Sprintf("%d_%s:%s", timestamp, cCode1, cCode2),
		Data: orderBookPair{
			Pair: []string{cCode1, cCode2},
		},
	}

	err := a.conn.WriteJSON(req)
	if err != nil {
		return err
	}

	msg := (<-sub).([]byte)

	resp := &responseOrderBookUnsubscribe{}

	err = json.Unmarshal(msg, resp)
	if err != nil {
		return err
	}

	if resp.OK != "ok" {
		return errors.New(resp.Data.Error)
	}

	handlerIdentifier := fmt.Sprintf("order-book-subscribe_%s:%s", cCode1, cCode2)

	// stop processing book messages
	a.orderBookHandlers[handlerIdentifier] <- true
	delete(a.orderBookHandlers, handlerIdentifier)

	return nil
}

func (a *API) responseCollector() {
	defer a.Close()

	resp := &responseAction{}

	for {
		_, msg, err := a.conn.ReadMessage()
		if err != nil {
			a.Logger.Println(err)
			return
		}

		err = json.Unmarshal(msg, resp)
		if err != nil {
			a.Logger.Printf("responseCollector: %s\nData: %s\n", err, string(msg))
			continue
		}

		subscriberIdentifier := resp.Action

		if resp.Action == "ping" {
			a.pong()
			continue
		}

		if resp.Action == "disconnecting" {
			a.Logger.Println(string(msg))
			break
		}

		if resp.Action == "order-book-subscribe" {
			ob := &responseOrderBookSubscribe{}
			err = json.Unmarshal(msg, ob)
			if err != nil {
				a.Logger.Printf("responseCollector | order-book-subscribe: %s\nData: %s\n", err, string(msg))
				continue
			}

			subscriberIdentifier = fmt.Sprintf("order-book-subscribe_%s", ob.Data.Pair)

			sub, err := a.subscriber(subscriberIdentifier)
			if err != nil {
				a.Logger.Printf("No response handler for message: %s", string(msg))
				continue // don't know how to handle message so just skip it
			}

			sub <- ob
			continue
		}

		if resp.Action == "md_update" {

			ob := &responseOrderBookUpdate{}
			err = json.Unmarshal(msg, ob)
			if err != nil {
				a.Logger.Printf("responseCollector | md_update: %s\nData: %s\n", err, string(msg))
				continue
			}

			subscriberIdentifier = fmt.Sprintf("order-book-subscribe_%s", ob.Data.Pair)

			sub, err := a.subscriber(subscriberIdentifier)
			if err != nil {
				a.Logger.Printf("No response handler for message: %s", string(msg))
				continue // don't know how to handle message so just skip it
			}

			sub <- ob
			continue
		}

		sub, err := a.subscriber(subscriberIdentifier)
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
	respMsg := (<-sub).([]byte)

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

func (a *API) pong() {

	msg := requestPong{"pong"}

	err := a.conn.WriteJSON(msg)
	if err != nil {
		a.Logger.Printf("Error while sending Pong message: %s", err)
	}
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
