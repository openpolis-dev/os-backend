package service

import (
	"testing"

	"github.com/theseed-labs/os-backend/internal/model"
)

func TestGetMySnsInviteCode(t *testing.T) {
	// ---> prepare
	truncateTable(model.TableSnsInviteCode)
	truncateTable(model.TableSnsInviteRecord)

	type args struct {
		userWallet string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "ok",
			args: args{
				userWallet: wallet1,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMySnsInviteCode(conn, tt.args.userWallet)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMySnsInviteCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			t.Logf("invite code: %s", got)
		})
	}
}

func TestSnsInvitedBy(t *testing.T) {
	// ---> prepare
	truncateTable(model.TableSnsInviteCode)
	truncateTable(model.TableSnsInviteRecord)

	wallet1Code, _ := GetMySnsInviteCode(conn, wallet1)
	err := SnsInvitedBy(conn, wallet1Code, wallet2)
	if err != nil {
		t.Errorf("SnsInvitedBy() error = %v", err)
		return
	}

	records, err := model.SnsInviteModel.FindInviteRecordByInviteUserWallet(conn, wallet1)
	if err != nil {
		t.Errorf("FindInviteRecordByInviteUserWallet() error = %v", err)
		return
	}
	if len(records) != 1 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records = %v, want 1", len(records))
		return
	}
	if records[0].InviteCode != wallet1Code {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteCode = %s, want %s", records[0].InviteCode, wallet2)
		return
	}
	if records[0].InviteeUserWallet != wallet2 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteeUserWallet = %s, want %s", records[0].InviteeUserWallet, wallet2)
		return
	}
	if records[0].SCRRewards.String() != "100" {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].SCRRewards = %s, want 100", records[0].SCRRewards.String())
		return
	}

	err = SnsInvitedBy(conn, wallet1Code, wallet2)
	if err == nil || err.Error() != "already invited" {
		t.Errorf("SnsInvitedBy() error = %v, want already invited", err)
		return
	}
}

func TestGetMySnsInviteRewards(t *testing.T) {
	// ---> prepare
	truncateTable(model.TableSnsInviteCode)
	truncateTable(model.TableSnsInviteRecord)

	wallet1Code, _ := GetMySnsInviteCode(conn, wallet1)
	wallet2Code, _ := GetMySnsInviteCode(conn, wallet2)

	_ = SnsInvitedBy(conn, wallet1Code, wallet2)
	_ = SnsInvitedBy(conn, wallet1Code, wallet3)

	count1, rewards1, err := GetMySnsInviteRewards(conn, wallet1)
	if count1 != 200 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 200", count1)
		return
	}
	if err != nil {
		t.Errorf("GetMySnsInviteRewards() error = %v", err)
		return
	}
	if rewards1.String() != "200" {
		t.Errorf("GetMySnsInviteRewards() rewards = %s, want 200", rewards1.String())
		return
	}

	_ = SnsInvitedBy(conn, wallet2Code, wallet4)
	_ = SnsInvitedBy(conn, wallet2Code, wallet5)

	count11, rewards11, _ := GetMySnsInviteRewards(conn, wallet1)
	if count11 != 200 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 200", count11)
		return
	}
	if rewards11.String() != "200" {
		t.Errorf("GetMySnsInviteRewards() rewards = %s, want 200", rewards11.String())
		return
	}
	count2, rewards2, _ := GetMySnsInviteRewards(conn, wallet2)
	if count2 != 200 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 200", count2)
		return
	}
	if rewards2.String() != "200" {
		t.Errorf("GetMySnsInviteRewards() rewards = %s, want 200", rewards11.String())
		return
	}
}
