package metaforo

import "errors"

var (
	NoLogin       = errors.New("please login")
	GroupNotExist = errors.New("group not exist")
	NoVoteRight   = errors.New("INCARNA NFT is required to perform this action")
	MetaforoError = errors.New("mataforo error")
)
