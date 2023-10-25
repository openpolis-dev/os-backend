package sdk

type Pusher interface {
	PushToWallets(ids []string, title map[string]string, body map[string]string, payload map[string]any) error
	PushAll(title map[string]string, body map[string]string, payload map[string]any) error
}

type PushToWalletsReq struct {
	Wallets []string    `json:"wallets"`
	Data    PushReqData `json:"data"`
}

type PushReqData struct {
	Title   map[string]string `json:"title"`
	Body    map[string]string `json:"body"`
	Payload map[string]any    `json:"payload"`
}
