package seeauth_inject

import (
	seeauth "github.com/Taoist-Labs/see-auth-go"
	user_inject "github.com/theseed-labs/os-backend/internal_inject/user"
)

type RefreshNonceReply struct {
	Nonce string `json:"nonce"`
}

type LoginWithSeeAuthReply struct {
	Token    string            `json:"token"`
	TokenExp int64             `json:"token_exp"` // time unit: seconds
	User     *user_inject.User `json:"user"`
	SEEAuth  *seeauth.SeeAuth  `json:"see_auth"`
}
