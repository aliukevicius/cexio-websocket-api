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
	stopDataCollector   bool

	//Dialer used to connect to WebSocket server
	Dialer *websocket.Dialer
	//Logger used for error logging
	Logger *log.Logger

	//ReceiveDone send message after Close() initiation
	ReceiveDone chan bool
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
		stopDataCollector:   false,
		ReceiveDone:         make(chan bool),
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

	a.stopDataCollector = true

	err := a.conn.Close()
	if err != nil {
		return err
	}

	go func() {
		a.ReceiveDone <- true
	}()

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
func (a *API) OrderBookSubscribe(cCode1 string, cCode2 string, depth int64, handler SubscriptionHandler) (int64, error) {

	action := "order-book-subscribe"

	currencyPair := fmt.Sprintf("%s:%s", cCode1, cCode2)

	subscriptionIdentifier := fmt.Sprintf("%s_%s", action, currencyPair)

	sub := a.subscribe(subscriptionIdentifier)
	defer a.unsubscribe(subscriptionIdentifier)

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
		return 0, err
	}

	bookSnapshot := (<-sub).(*responseOrderBookSubscribe)

	go a.handleOrderBookSubscriptions(bookSnapshot, currencyPair, handler)

	return bookSnapshot.Data.ID, nil
}

func (a *API) handleOrderBookSubscriptions(bookSnapshot *responseOrderBookSubscribe, currencyPair string, handler SubscriptionHandler) {

	quit := make(chan bool)

	subscriptionIdentifier := fmt.Sprintf("md_update_%s", currencyPair)
	a.subscribe(subscriptionIdentifier)
	a.orderBookHandlers[subscriptionIdentifier] = quit

	obData := OrderBookUpdateData{
		ID:        bookSnapshot.Data.ID,
		Pair:      bookSnapshot.Data.Pair,
		Timestamp: bookSnapshot.Data.Timestamp,
		Bids:      bookSnapshot.Data.Bids,
		Asks:      bookSnapshot.Data.Asks,
	}

	// process order book snapshot items before order book updates
	go handler(obData)

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

			resp := m.(*responseOrderBookUpdate)

			obData := OrderBookUpdateData{
				ID:        resp.Data.ID,
				Pair:      resp.Data.Pair,
				Timestamp: resp.Data.Timestamp,
				Bids:      resp.Data.Bids,
				Asks:      resp.Data.Asks,
			}

			go handler(obData)
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

	handlerIdentifier := fmt.Sprintf("md_update_%s:%s", cCode1, cCode2)

	// stop processing book messages
	a.orderBookHandlers[handlerIdentifier] <- true
	delete(a.orderBookHandlers, handlerIdentifier)

	return nil
}

func (a *API) responseCollector() {
	defer a.Close()

	a.stopDataCollector = false

	resp := &responseAction{}

	for a.stopDataCollector == false {
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

			subscriberIdentifier = fmt.Sprintf("md_update_%s", ob.Data.Pair)

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
