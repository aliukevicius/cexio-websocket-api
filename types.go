package cexio

type subscriberType interface{}

type requestAuthAction struct {
	E    string          `json:"e"`
	Auth requestAuthData `json:"auth"`
}

type requestAuthData struct {
	Key       string `json:"key"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
}

type responseAction struct {
	Action string `json:"e"`
}

type responseAuth struct {
	E    string           `json:"e"`
	Data responseAuthData `json:"data"`
	OK   string           `json:"ok"`
}

type responseAuthData struct {
	Error string `json:"error"`
	OK    string `json:"ok"`
}

type requestPong struct {
	E string `json:"e"`
}

type requestTicker struct {
	E    string   `json:"e"`
	Data []string `json:"data"`
	Oid  string   `json:"oid"`
}

type responseTicker struct {
	E    string             `json:"e"`
	Data responseTickerData `json:"data"`
	OK   string             `json:"ok"`
	Oid  string             `json:"oid"`
}

type responseTickerData struct {
	Bid   float64  `json:"bid"`
	Ask   float64  `json:"ask"`
	Pair  []string `json:"pair"`
	Error string   `json:"error"`
}

type requestOrderBookSubscribe struct {
	E    string                        `json:"e"`
	Data requestOrderBookSubscribeData `json:"data"`
	Oid  string                        `json:"oid"`
}

type requestOrderBookSubscribeData struct {
	Pair      []string `json:"pair"`
	Subscribe bool     `json:"subscribe"`
	Depth     int64    `json:"depth"`
}

type responseOrderBookSubscribe struct {
	E    string                         `json:"e"`
	Data responseOrderBookSubscribeData `json:"data"`
	OK   string                         `json:"ok"`
	Oid  string                         `json:"oid"`
}

type responseOrderBookSubscribeData struct {
	Timestamp int64       `json:"timestamp"`
	Bids      [][]float64 `json:"bids"`
	Asks      [][]float64 `json:"asks"`
	Pair      string      `json:"pair"`
	ID        int64       `json:"id"`
}

type responseOrderBookUpdate struct {
	E    string                      `json:"e"`
	Data responseOrderBookUpdateData `json:"data"`
}

type responseOrderBookUpdateData struct {
	ID        int64       `json:"id"`
	Pair      string      `json:"pair"`
	Timestamp int64       `json:"time"`
	Bids      [][]float64 `json:"bids"`
	Asks      [][]float64 `json:"asks"`
}

//OrderBookUpdateData data of order book update
type OrderBookUpdateData struct {
	ID        int64
	Pair      string
	Timestamp int64
	Bids      [][]float64
	Asks      [][]float64
}

//SubscriptionHandler subscription update handler type
type SubscriptionHandler func(updateData OrderBookUpdateData)

type orderBookPair struct {
	Pair  []string `json:"pair"`
	Error string   `json:"error,omitempty"`
}

type requestOrderBookUnsubscribe struct {
	E    string        `json:"e"`
	Data orderBookPair `json:"data"`
	Oid  string        `json:"oid"`
}

type responseOrderBookUnsubscribe struct {
	E    string        `json:"e"`
	Data orderBookPair `json:"data"`
	OK   string        `json:"ok"`
	Oid  string        `json:"oid"`
}
