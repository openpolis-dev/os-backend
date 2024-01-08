package metaforo

import (
	"errors"
	"strconv"
	"testing"
)

func TestDeleteCategory(t *testing.T) {
	type args struct {
		accessToken string
		groupName   string
		categoryId  string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name:    "success",
			args:    args{accessToken: token, groupName: groupName2, categoryId: strconv.Itoa(categoryId2)},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := DeleteCategory(tt.args.accessToken, tt.args.groupName, tt.args.categoryId); !errors.Is(err, tt.wantErr) {
				t.Errorf("DeleteCategory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetCategories(t *testing.T) {
	type args struct {
		groupName string
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name:    "success",
			args:    args{groupName: groupName2},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCategories(tt.args.groupName)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("GetCategories() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			t.Logf("%v", got)
		})
	}
}

func TestNewCategory(t *testing.T) {
	type args struct {
		accessToken    string
		groupName      string
		categoryName   string
		iconUnicode    string
		parentId       string
		templateId     string
		permissionList []*NewCategoryPermissionRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "success",
			args: args{
				accessToken:  token,
				groupName:    groupName2,
				categoryName: "testCategory",
				iconUnicode:  "",
				parentId:     "1",
				templateId:   "0",
				permissionList: []*NewCategoryPermissionRequest{
					{
						Id:        0,
						Name:      "Everyone",
						CanSee:    1,
						CanReply:  1,
						CanCreate: 1,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := NewCategory(tt.args.accessToken, tt.args.groupName, tt.args.categoryName, tt.args.iconUnicode, tt.args.parentId, tt.args.templateId, tt.args.permissionList); !errors.Is(err, tt.wantErr) {
				t.Errorf("NewCategory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateCategory(t *testing.T) {
	type args struct {
		accessToken    string
		groupName      string
		categoryId     string
		categoryName   string
		iconUnicode    string
		parentId       string
		templateId     string
		permissionList []*NewCategoryPermissionRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "success",
			args: args{
				accessToken:  token,
				groupName:    groupName2,
				categoryId:   strconv.Itoa(categoryId2),
				categoryName: "testCategory22",
				iconUnicode:  "",
				parentId:     "1",
				templateId:   "0",
				permissionList: []*NewCategoryPermissionRequest{
					{
						Id:        0,
						Name:      "Everyone",
						CanSee:    1,
						CanReply:  1,
						CanCreate: 1,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := UpdateCategory(tt.args.accessToken, tt.args.groupName, tt.args.categoryId, tt.args.categoryName, tt.args.iconUnicode, tt.args.parentId, tt.args.templateId, tt.args.permissionList); !errors.Is(err, tt.wantErr) {
				t.Errorf("UpdateCategory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
