package metaforo

import "errors"

var (
	NoLogin       = errors.New("please login")
	GroupNotExist = errors.New("group not exist")
	BadReq        = errors.New("bad request")
	MetaforoError = errors.New("mataforo error")
)
