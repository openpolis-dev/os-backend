package metaforo

import (
	"errors"
	"strconv"
	"testing"
)

func TestCreateNftGate(t *testing.T) {
	tokenAddress := "0x6811f2f20c42f42656a3c8623ad5e9461b83f719"
	type args struct {
		accessToken string
		groupName   string
		chainType   string
		tokenType   string
		alias       string
		address     string
		tokenId     string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "success",
			args: args{
				accessToken: token,
				groupName:   groupName2,
				chainType:   "1",
				tokenType:   "2",
				alias:       "ETH-ERC1155",
				address:     tokenAddress,
				tokenId:     "1",
			},
			wantErr: nil,
		},
		{
			name: "success",
			args: args{
				accessToken: token,
				groupName:   groupName2,
				chainType:   "9",
				tokenType:   "2",
				alias:       "ETH-ERC1155",
				address:     tokenAddress,
				tokenId:     "1",
			},
			wantErr: TokenAddrInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CreateNftGate(tt.args.accessToken, tt.args.groupName, tt.args.chainType, tt.args.tokenType, tt.args.alias, tt.args.address, tt.args.tokenId); !errors.Is(err, tt.wantErr) {
				t.Errorf("CreateNftGate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteNftGate(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		nftGateId   string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "success",
			args: args{
				accessToken: token,
				groupName:   groupName2,
				nftGateId:   strconv.Itoa(gateTokenId2),
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := DeleteNftGate(tt.args.accessToken, tt.args.groupName, tt.args.nftGateId); !errors.Is(err, tt.wantErr) {
				t.Errorf("DeleteNftGate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
