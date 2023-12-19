package metaforo

import (
	"errors"
	"testing"
)

func TestAddComment(t *testing.T) {
	type args struct {
		accessToken string
		content     string
		proposalId  string
		groupName   string
		replyId     *string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "comment: success",
			args: args{
				accessToken: token,
				content:     commentContent,
				proposalId:  proposalId,
				groupName:   groupName,
				replyId:     nil,
			},
			wantErr: nil,
		},
		{
			name: "comment: thread not exist",
			args: args{
				accessToken: token,
				content:     commentContent,
				proposalId:  "479674796747967",
				groupName:   groupName,
			},
			wantErr: MetaforoError,
		},
		{
			name: "comment: not login",
			args: args{
				accessToken: "21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1Fmffgnsss",
				content:     commentContent,
				proposalId:  proposalId,
				groupName:   groupName,
			},
			wantErr: NoLogin,
		},
		{
			name: "reply comment: success",
			args: args{
				accessToken: token,
				content:     commentContent,
				proposalId:  proposalId,
				groupName:   groupName,
				replyId:     &commentId,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		if err := AddComment(tt.args.accessToken, tt.args.content, tt.args.proposalId, tt.args.groupName, tt.args.replyId); !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] AddComment() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}
	}
}
