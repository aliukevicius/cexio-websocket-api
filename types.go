package cexio

type requestAuthAction struct {
	E    string          `json:"e"`
	Auth requestAuthData `json:"auth"`
}

type requestAuthData struct {
	Key       string `json:"key"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
}

type response struct {
	E    string           `json:"e"`
	Data responseAuthData `json:"data"`
	OK   string           `json:"ok"`
}

type responseAuthData struct {
	Error string `json:"error"`
	OK    string `json:"ok"`
}
