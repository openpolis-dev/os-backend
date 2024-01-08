package metaforo

import (
	"errors"
	"strconv"
	"testing"
)

func TestAddTag(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		tagName     string
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
				tagName:     "testTag",
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := AddTag(tt.args.accessToken, tt.args.groupName, tt.args.tagName); !errors.Is(err, tt.wantErr) {
				t.Errorf("AddTag() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteTag(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		tagId       string
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
				tagId:       strconv.Itoa(tagId2),
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := DeleteTag(tt.args.accessToken, tt.args.groupName, tt.args.tagId); !errors.Is(err, tt.wantErr) {
				t.Errorf("DeleteTag() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateTag(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		tagId       string
		newTagName  string
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
				tagId:       strconv.Itoa(tagId2),
				newTagName:  "testTag22",
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := UpdateTag(tt.args.accessToken, tt.args.groupName, tt.args.tagId, tt.args.newTagName); !errors.Is(err, tt.wantErr) {
				t.Errorf("UpdateTag() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetTags(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
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
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTags(tt.args.accessToken, tt.args.groupName)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("getTags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			t.Logf("%v", got)
		})
	}
}
