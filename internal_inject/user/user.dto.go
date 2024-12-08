package user_inject

import (
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type RefreshNonceReq struct {
	Wallet string `json:"wallet"`
}

type RefreshNonceReply struct {
	Nonce string `json:"nonce"`
}

type LoginReq struct {
	Wallet         string `json:"wallet" binding:"required"`
	WalletType     string `json:"wallet_type" binding:"required"`
	IsEIP191Prefix bool   `json:"is_eip191_prefix"`
	Domain         string `json:"domain" binding:"required"`
	Message        string `json:"message" binding:"required"`
	Signature      string `json:"signature" binding:"required"`
}

type LoginReply struct {
	Token    string      `json:"token"`
	TokenExp int64       `json:"token_exp"` // time unit: seconds
	User     *model.User `json:"user"`

	UserVerified bool `json:"user_verified"` // for unipass user,if wallet signature not verified, will be false
}

type UpdateReq struct {
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Bio            string `json:"bio"`
	Email          string `json:"email"`
	Wechat         string `json:"wechat"`
	DiscordProfile string `json:"discord_profile"`
	TwitterProfile string `json:"twitter_profile"`
	GoogleProfile  string `json:"google_profile"`
	GithubProfile  string `json:"github_profile"`
	Mirror         string `json:"mirror"`
}

type UserModelWithSomeSeepassData struct {
	model.User
	Sp *sdk.SeepassResponse `json:"sp"`
}

type UserLvlRes struct {
	CurrentLv string `json:"current_lv"`
}

type JoinOrLeaveGroupReq struct {
	GroupName           string `json:"group_name"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type MetaforoActivityRecord struct {
	Wallet           string `json:"wallet"`
	ProposalID       uint   `json:"proposal_id"`
	Action           string `json:"metaforo_action"`
	ThreadTitle      string `json:"target_title"`       // Thread name this action is performed on
	MetaforoThreadId int    `json:"metaforo_thread_id"` // Metaforo thread id
	ReplyToWallet    string `json:"reply_to_wallet"`    // User this action is performed on
	ActionTs         int64  `json:"action_ts"`
}

type MetaforoActivityResponse struct {
	Session string                    `json:"session"`
	Records []*MetaforoActivityRecord `json:"records"`
}
