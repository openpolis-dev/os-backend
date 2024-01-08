package metaforo

import (
	"errors"
	"testing"
)

func TestLogin(t *testing.T) {
	wallet := "0x41d2ce62cd81d9ccd5c6890dcb44267b26165f85"
	sign := "0x9f879aef76793f50e1844ef8f425dc8840f00fe1bd6f0c27707c5b75445523a21d06354616b85e498b9f41de353d104855add16b631f13c1062862b4365b9b9b1b"
	signMsg := `{"domain":{"chainId":"1","name":"https://www.metaforo.io","version":"1"},"message":{"message":"To sign in to Metaforo please sign your request to prove your identity. This is not a transaction.","timeStamp":"1703080852021","signContent":"\"\"","URI":"https://www.metaforo.io","Version":"1","ChainID":"1"},"primaryType":"Post","types":{"EIP712Domain":[{"name":"name","type":"string"},{"name":"version","type":"string"},{"name":"chainId","type":"string"}],"Post":[{"name":"Version","type":"string"},{"name":"URI","type":"string"},{"name":"ChainID","type":"string"},{"name":"message","type":"string"},{"name":"signContent","type":"string"},{"name":"timeStamp","type":"string"}]}}`
	type args struct {
		groupName  string
		wallet     string
		sign       string
		signMsg    string
		walletType string
	}
	tests := []struct {
		name     string
		args     args
		wantResp *LoginResponse
		wantErr  error
	}{
		{
			name: "success",
			args: args{
				groupName:  groupName,
				wallet:     wallet,
				sign:       sign,
				signMsg:    signMsg,
				walletType: "5",
			},
			wantResp: &LoginResponse{
				User: &UserDetailResponse{
					Web3PublicKey: wallet,
				},
			},
			wantErr: nil,
		},
		{
			name: "login failed",
			args: args{
				groupName:  groupName,
				wallet:     "0x41d2ce62cd81d9ccd5c6890dcb44267b26165f88",
				sign:       sign,
				signMsg:    signMsg,
				walletType: "5",
			},
			wantResp: nil,
			wantErr:  SignError,
		},
	}
	for _, tt := range tests {
		gotResp, err := Login(tt.args.groupName, tt.args.wallet, tt.args.sign, tt.args.signMsg, tt.args.walletType)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] Login() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}

		if tt.wantResp == nil {
			if gotResp != nil {
				t.Errorf("[%s] Login() gotResp = %v, wantResp = %v", tt.name, gotResp, tt.wantResp)
			}
		} else {
			if gotResp == nil {
				t.Errorf("[%s] Login() gotResp = %v, wantResp = %v", tt.name, gotResp, tt.wantResp)
			}

			if gotResp.ApiToken == "" {
				t.Errorf("[%s] Login() gotResp.ApiToken is empty", tt.name)
			}
			if gotResp.User.Web3PublicKey != tt.args.wallet {
				t.Errorf("[%s] Login() gotResp.User.Web3PublicKey = %v, wantResp.User.Web3PublicKey = %v", tt.name, gotResp.User.Web3PublicKey, tt.args.wallet)
			}
		}
	}
}
