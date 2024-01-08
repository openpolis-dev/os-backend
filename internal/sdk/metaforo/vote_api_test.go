package metaforo

import (
	"errors"
	"testing"
)

const (
	pollId         = 1734
	optionId       = 4806
	failedPollId   = 1730
	failedOptionId = 4797
)

func TestCastVote(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		pollId      int
		options     []int
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
				pollId:      pollId,
				options:     []int{optionId},
			},
			wantErr: nil,
		},
		{
			name: "no vote right",
			args: args{
				accessToken: token,
				groupName:   groupName2,
				pollId:      failedPollId,
				options:     []int{failedOptionId},
			},
			wantErr: NoVoteRight,
		},
	}
	for _, tt := range tests {
		if err := CastVote(tt.args.accessToken, tt.args.groupName, tt.args.pollId, tt.args.options); !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] CastVote() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestRevokeVote(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		pollId      int
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
				pollId:      pollId,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		if err := RevokeVote(tt.args.accessToken, tt.args.groupName, tt.args.pollId); !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] RevokeVote() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}
	}
}
