package metaforo

import (
	"errors"
	"testing"
)

func TestGetGroupInfo(t *testing.T) {
	type resp struct {
		Id    int
		Name  string
		Title string
	}
	tests := []struct {
		name      string
		groupName string
		wantErr   error
		wantResp  *resp
	}{
		{
			name:      "success",
			groupName: groupName,
			wantErr:   nil,
			wantResp: &resp{
				Id:    10434,
				Name:  "testttt",
				Title: "test ttt",
			},
		},
		{
			name:      "group not exist",
			groupName: "testtttxx",
			wantErr:   GroupNotExist,
			wantResp:  nil,
		},
	}
	for _, tt := range tests {
		gotResp, err := GetGroupInfo(tt.groupName)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] GetGroupInfo() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}

		if tt.wantResp == nil {
			if gotResp != nil {
				t.Errorf("[%s] GetGroupInfo() gotResp = %v, wantResp = %v", tt.name, gotResp, tt.wantResp)
			}
		} else {
			if gotResp == nil {
				t.Errorf("[%s] GetGroupInfo() gotResp = %v, wantResp = %v", tt.name, gotResp, tt.wantResp)
			}

			if gotResp.Id != tt.wantResp.Id {
				t.Errorf("[%s] GetGroupInfo() gotResp.Id = %v, wantResp.Id = %v", tt.name, gotResp.Id, tt.wantResp.Id)
			}
			if gotResp.Name != tt.wantResp.Name {
				t.Errorf("[%s] GetGroupInfo() gotResp.Name = %v, wantResp.Name = %v", tt.name, gotResp.Name, tt.wantResp.Name)
			}
			if gotResp.Title != tt.wantResp.Title {
				t.Errorf("[%s] GetGroupInfo() gotResp.Title = %v, wantResp.Title = %v", tt.name, gotResp.Title, tt.wantResp.Title)
			}
		}
	}
}
