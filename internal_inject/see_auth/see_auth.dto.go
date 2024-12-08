package seeauth_inject

import (
	seeauth "github.com/Taoist-Labs/see-auth-go"
	"github.com/theseed-labs/os-backend/internal/model"
)

type RefreshNonceReply struct {
	Nonce string `json:"nonce"`
}

type LoginWithSeeAuthReply struct {
	Token    string           `json:"token"`
	TokenExp int64            `json:"token_exp"` // time unit: seconds
	User     *model.User      `json:"user"`
	SEEAuth  *seeauth.SeeAuth `json:"see_auth"`
}
