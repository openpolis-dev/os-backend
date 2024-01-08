package metaforo

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"
)

func TestCreateProposal(t *testing.T) {
	type args struct {
		accessToken     string
		groupName       string
		categoryIndexId string
		title           string
		content         string
		tags            []*NewProposalTagRequest
		polls           []*NewVoteFormRequest
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "no vote",
			args: args{
				accessToken:     token,
				groupName:       groupName2,
				categoryIndexId: categoryIndexId2,
				title:           proposalTitle,
				content:         proposeContent,
				tags: []*NewProposalTagRequest{
					{
						Id:   tagId2,
						Name: tagName2,
					},
				},
			},
		},
		{
			name: "no vote+tag",
			args: args{
				accessToken:     token,
				groupName:       groupName2,
				categoryIndexId: categoryIndexId2,
				title:           proposalTitle,
				content:         proposeContent,
			},
		},
		{
			name: "every one can vote",
			args: args{
				accessToken:     token,
				groupName:       groupName2,
				categoryIndexId: categoryIndexId2,
				title:           proposalTitle,
				content:         proposeContent,
				tags: []*NewProposalTagRequest{
					{
						Id:   tagId2,
						Name: tagName2,
					},
				},
				polls: []*NewVoteFormRequest{
					{
						Options: []*VoteOption{
							{
								Text: "option1",
								Type: 1,
							},
							{
								Text: "option2",
								Type: 0,
							},
						},
						Type:               "1",
						Title:              "every one can vote",
						ShowType:           "1",
						ShowResult:         true,
						ChartType:          "1",
						VoteType:           "1",
						ChainType:          0,
						ContractType:       0,
						SettingId:          0,
						Period:             "1",
						CloseAt:            "2023-12-28 23:59:59",
						VoteStartAt:        "2023-12-20 00:00:00",
						Max:                1,
						MinTokens:          "0",
						PollCategory:       "0",
						LastCategroyChange: "0",
						TokenId:            0,
						Quorum:             false,
						Weight:             true,
						Step:               2,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		pollFormDataBytes, err := json.Marshal(tt.args.polls)
		if err != nil {
			t.Errorf("[%s] CreateProposal() marshal form bytes error = %v", tt.name, err)
		}
		got, err := CreateProposal(tt.args.accessToken, tt.args.groupName, tt.args.categoryIndexId, tt.args.title, tt.args.content, tt.args.tags, string(pollFormDataBytes))
		if err != nil {
			t.Errorf("[%s] CreateProposal() error = %v", tt.name, err)
		}

		if got.Thread.Title != tt.args.title {
			t.Errorf("[%s] CreateProposal() got.Title = %v, want %v", tt.name, got.Thread.Title, tt.args.title)
		}
		if got.Thread.GroupId != groupId2 {
			t.Errorf("[%s] CreateProposal() got.groupId = %v, want %v", tt.name, got.Thread.GroupId, groupId2)
		}
	}
}

func TestDeleteProposal(t *testing.T) {
	got, err := CreateProposal(token, groupName2, categoryIndexId2, proposalTitle, proposeContent, nil, "")
	if err != nil {
		t.Errorf("CreateProposal() error = %v", err)
	}

	t.Logf("delete proposal id=%d", got.Thread.FirstPostId)

	err = DeleteProposal(token, strconv.Itoa(got.Thread.FirstPostId), groupName2)
	if err != nil {
		t.Errorf("DeleteProposal() error = %v", err)
	}
}

func TestGetProposals(t *testing.T) {
	type resp struct {
		Id      int    `json:"id"`
		Title   string `json:"title"`
		GroupId int    `json:"group_id"`
	}
	type args struct {
		proposalId int
		groupName  string
	}

	tests := []struct {
		name     string
		args     args
		wantErr  error
		wantResp *resp
	}{
		{
			name: "success",
			args: args{
				proposalId: proposalId,
				groupName:  groupName,
			},
			wantErr: nil,
			wantResp: &resp{
				Id:      47967,
				Title:   "测试评论",
				GroupId: 10434,
			},
		},
		{
			name: "proposal not exist",
			args: args{
				proposalId: 479674796747967,
				groupName:  groupName,
			},
			wantErr:  MetaforoError,
			wantResp: nil,
		},
	}
	for _, tt := range tests {
		gotResp, err := GetProposal(tt.args.proposalId, tt.args.groupName, "", 0)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] GetProposal() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}

		if tt.wantResp == nil {
			if gotResp != nil {
				t.Errorf("[%s] GetProposal() gotResp = %v, wantResp = %v", tt.name, gotResp, tt.wantResp)
			}
		} else {
			if gotResp == nil {
				t.Errorf("[%s] GetProposal() gotResp = %v, wantResp = %v", tt.name, gotResp, tt.wantResp)
			}

			if gotResp.Thread.Id != tt.wantResp.Id {
				t.Errorf("[%s] GetProposal() gotResp.Id = %v, wantResp.Id = %v", tt.name, gotResp.Thread.Id, tt.wantResp.Id)
			}
			if gotResp.Thread.Title != tt.wantResp.Title {
				t.Errorf("[%s] GetProposal() gotResp.Title = %v, wantResp.Title = %v", tt.name, gotResp.Thread.Title, tt.wantResp.Title)
			}
			if gotResp.Thread.GroupId != tt.wantResp.GroupId {
				t.Errorf("[%s] GetGroupInfo() gotResp.GroupId = %v, wantResp.GroupId = %v", tt.name, gotResp.Thread.GroupId, tt.wantResp.GroupId)
			}
		}
	}
}

func TestListProposals(t *testing.T) {
	type args struct {
		paginationParams *PaginationParams
	}
	tests := []struct {
		name       string
		args       args
		wantErr    error
		wantLength int
	}{
		{
			name: "success",
			args: args{paginationParams: &PaginationParams{
				Page:            1,
				PerPage:         1,
				Filter:          "all",
				CategoryIndexId: 0,
				TagId:           0,
				Sort:            "new",
				GroupName:       groupName,
			}},
			wantErr:    nil,
			wantLength: 1,
		},
		{
			name: "category not exist",
			args: args{paginationParams: &PaginationParams{
				Page:            1,
				PerPage:         1,
				Filter:          "all",
				CategoryIndexId: 99999,
				TagId:           0,
				Sort:            "new",
				GroupName:       groupName,
			}},
			wantErr:    nil,
			wantLength: 0,
		},
		{
			name: "tag not exist",
			args: args{paginationParams: &PaginationParams{
				Page:            1,
				PerPage:         1,
				Filter:          "all",
				CategoryIndexId: 0,
				TagId:           99999,
				Sort:            "new",
				GroupName:       groupName,
			}},
			wantErr:    nil,
			wantLength: 0,
		},
	}
	for _, tt := range tests {
		got, err := ListProposals(tt.args.paginationParams)

		if !errors.Is(err, tt.wantErr) {
			t.Errorf("[%s] ListProposals() error = %v, wantErr = %v", tt.name, err, tt.wantErr)
		}
		if len(got) != tt.wantLength {
			t.Errorf("[%s] ListProposals() got = %v, want %v", tt.name, len(got), tt.wantLength)
		}
	}
}
