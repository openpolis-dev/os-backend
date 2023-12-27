package metaforo

import (
	"errors"
	"testing"
)

func TestAddComment(t *testing.T) {
	type args struct {
		accessToken string
		content     []*NewContentRequest
		proposalId  int
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
				content:     []*NewContentRequest{&commentContent},
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
				content:     []*NewContentRequest{&commentContent},
				proposalId:  479674796747967,
				groupName:   groupName,
			},
			wantErr: MetaforoError,
		},
		{
			name: "comment: not login",
			args: args{
				accessToken: "21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1Fmffgnsss",
				content:     []*NewContentRequest{&commentContent},
				proposalId:  proposalId,
				groupName:   groupName,
			},
			wantErr: NoLogin,
		},
		{
			name: "reply comment: success",
			args: args{
				accessToken: token,
				content:     []*NewContentRequest{&commentContent},
				proposalId:  proposalId,
				groupName:   groupName,
				replyId:     &commentId,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := AddComment(tt.args.accessToken, tt.args.groupName, tt.args.proposalId, tt.args.content, tt.args.replyId); !errors.Is(err, tt.wantErr) {
				t.Errorf("[%s] AddComment() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestEditComment(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		postId      string
		content     []*NewContentRequest
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
				postId:      commentId,
				content:     []*NewContentRequest{&commentContent},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := EditComment(tt.args.accessToken, tt.args.groupName, tt.args.postId, tt.args.content); !errors.Is(err, tt.wantErr) {
				t.Errorf("EditComment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteComment(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		postId      string
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
				postId:      commentId,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := DeleteComment(tt.args.accessToken, tt.args.groupName, tt.args.postId); !errors.Is(err, tt.wantErr) {
				t.Errorf("DeleteComment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
