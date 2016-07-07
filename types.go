package cexio

type subscriberType []byte

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
