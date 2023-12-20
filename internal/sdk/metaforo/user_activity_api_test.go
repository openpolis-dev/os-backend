package metaforo

import (
	"errors"
	"testing"
)

func TestUserActivities(t *testing.T) {
	userId := "39175"

	type args struct {
		accessToken string
		userId      string
		filter      string
		perPage     string
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
				userId:      userId,
				filter:      "all",
				perPage:     "1",
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		got, err := UserActivities(tt.args.accessToken, tt.args.userId, tt.args.filter, tt.args.perPage)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] UserActivities() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}
		if len(got) != 1 {
			t.Errorf("[%s] UserActivities() got = %v, want = %v", tt.name, len(got), 1)
		}
	}
}
