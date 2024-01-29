package metaforo

import "errors"

var (
	NoLogin          = errors.New("please login")
	SignError        = errors.New("sign error")
	GroupNotExist    = errors.New("group not exist")
	NoVoteRight      = errors.New("INCARNA NFT is required to perform this action")
	TokenAddrInvalid = errors.New("token address is invalid")
	MetaforoError    = errors.New("mataforo error")
)
