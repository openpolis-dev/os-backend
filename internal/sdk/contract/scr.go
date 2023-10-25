package contract

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/theseed-labs/os-backend/internal/sdk/contract/generated"
)

func SCRTotalSupply(client *ethclient.Client, contractAddr string) (*big.Int, error) {
	c, err := generated.NewSCR(common.HexToAddress(contractAddr), client)
	if err != nil {
		return nil, err
	}

	return c.TotalSupply(nil)
}
