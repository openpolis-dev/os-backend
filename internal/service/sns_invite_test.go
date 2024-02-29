package service

import (
	"testing"
	"time"

	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
)

func TestGetMySnsInviteCode(t *testing.T) {
	t.Skip("skip this case for now since it can't be run in CI")
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
	t.Skip("skip this case for now since it can't be run in CI")
	// ---> prepare
	truncateTable(model.TableSnsInviteCode)
	truncateTable(model.TableSnsInviteRecord)

	wallet1Code, _ := GetMySnsInviteCode(conn, wallet1)

	// CASE: invite success
	err := SnsInvitedBy(conn, wallet1Code, wallet2)
	if err != nil {
		t.Errorf("SnsInvitedBy() error = %v", err)
		return
	}
	// verify result
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
	if records[0].InviteUserWallet != wallet1 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteUserWallet = %s, want %s", records[0].InviteUserWallet, wallet5)
		return
	}
	if records[0].InviteeUserWallet != wallet2 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteeUserWallet = %s, want %s", records[0].InviteeUserWallet, wallet2)
		return
	}
	if records[0].Verified {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].Verified = %v, want false", records[0].Verified)
		return
	}
	if records[0].SCRRewards.String() != "100" {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].SCRRewards = %s, want 100", records[0].SCRRewards.String())
		return
	}

	// CASE: can change invite data when not verified
	wallet5Code, _ := GetMySnsInviteCode(conn, wallet5)
	err = SnsInvitedBy(conn, wallet5Code, wallet2)
	// verify result
	records2, err := model.SnsInviteModel.FindInviteRecordByInviteUserWallet(conn, wallet5)
	if err != nil {
		t.Errorf("FindInviteRecordByInviteUserWallet() error = %v", err)
		return
	}
	if len(records2) != 1 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records = %v, want 1", len(records2))
		return
	}
	if records2[0].InviteCode != wallet5Code {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteCode = %s, want %s", records2[0].InviteCode, wallet2)
		return
	}
	if records2[0].InviteUserWallet != wallet5 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteUserWallet = %s, want %s", records2[0].InviteUserWallet, wallet5)
		return
	}
	if records2[0].InviteeUserWallet != wallet2 {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].InviteeUserWallet = %s, want %s", records2[0].InviteeUserWallet, wallet2)
		return
	}
	if records2[0].Verified {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].Verified = %v, want false", records2[0].Verified)
		return
	}
	if records2[0].SCRRewards.String() != "100" {
		t.Errorf("FindInviteRecordByInviteUserWallet() records[0].SCRRewards = %s, want 100", records2[0].SCRRewards.String())
		return
	}

	// CASE: can't change invite data when has verified
	// mark as verified
	_ = model.SnsInviteModel.UpdateInviteRecordVerified(conn, wallet2)
	err = SnsInvitedBy(conn, wallet1Code, wallet2)
	if err == nil || err.Error() != "already invited" {
		t.Errorf("SnsInvitedBy() error = %v, want already invited", err)
		return
	}
}

func TestGetMySnsInviteRewards(t *testing.T) {
	t.Skip("skip this case for now since it can't be run in CI")
	// ---> prepare
	truncateTable(model.TableSnsInviteCode)
	truncateTable(model.TableSnsInviteRecord)

	wallet1Code, _ := GetMySnsInviteCode(conn, wallet1)
	wallet2Code, _ := GetMySnsInviteCode(conn, wallet2)

	_ = SnsInvitedBy(conn, wallet1Code, wallet2)
	_ = SnsInvitedBy(conn, wallet1Code, wallet3)
	// mark as verified
	_ = model.SnsInviteModel.UpdateInviteRecordVerified(conn, wallet2)
	_ = model.SnsInviteModel.UpdateInviteRecordVerified(conn, wallet3)

	count1, rewards1, err := GetMySnsInviteRewards(conn, wallet1)
	if count1 != 2 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 2", count1)
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
	// mark as verified
	_ = model.SnsInviteModel.UpdateInviteRecordVerified(conn, wallet4)
	_ = model.SnsInviteModel.UpdateInviteRecordVerified(conn, wallet5)

	count11, rewards11, _ := GetMySnsInviteRewards(conn, wallet1)
	if count11 != 2 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 2", count11)
		return
	}
	if rewards11.String() != "200" {
		t.Errorf("GetMySnsInviteRewards() rewards = %s, want 200", rewards11.String())
		return
	}
	count2, rewards2, _ := GetMySnsInviteRewards(conn, wallet2)
	if count2 != 2 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 2", count2)
		return
	}
	if rewards2.String() != "200" {
		t.Errorf("GetMySnsInviteRewards() rewards = %s, want 200", rewards11.String())
		return
	}
}

func TestCheckAndUpdateUnverifiedSnsInvite(t *testing.T) {
	t.Skip("skip this case for now since it can't be run in CI")
	// ---> prepare
	truncateTable(model.TableSnsInviteCode)
	truncateTable(model.TableSnsInviteRecord)
	truncateTable("seasons")
	truncateTable("app_bundles")
	truncateTable("app_bundle_audit_logs")
	truncateTable("applications")
	truncateTable("application_audit_logs")

	// prepare config
	cfg := &config.Config{
		SnsInvite: config.SnsInvite{
			EntityType: "project",
			EntityId:   1,
			EntityName: "新手村",
			Applicant:  wallet1,
		},
	}

	now := time.Now().In(internal.ProjectTimezone).Unix()
	conn.Model(&model.Season{}).Create(&model.Season{Idx: 1, StartAt: now - 100, EndAt: now + 100})

	wallet1Code, _ := GetMySnsInviteCode(conn, wallet1)

	_ = SnsInvitedBy(conn, wallet1Code, "0x8C913aEc7443FE2018639133398955e0E17FB0C1") // has sns
	_ = SnsInvitedBy(conn, wallet1Code, wallet2)                                      // no sns

	// ---> test
	err := CheckAndUpdateUnverifiedSnsInvite(cfg, conn)
	if err != nil {
		t.Errorf("CheckAndUpdateUnverifiedSnsInvite() error = %v", err)
		return
	}

	count1, rewards1, err := GetMySnsInviteRewards(conn, wallet1)
	if err != nil {
		t.Errorf("GetMySnsInviteRewards() error = %v", err)
		return
	}
	if count1 != 1 {
		t.Errorf("GetMySnsInviteRewards() count = %d, want 1", count1)
		return
	}
	if rewards1.String() != "100" {
		t.Errorf("GetMySnsInviteRewards() rewards = %s, want 100", rewards1.String())
		return
	}
}
