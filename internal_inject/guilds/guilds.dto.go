package guilds_inject

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
)

type (
	CreateReq struct {
		LogoStr string `json:"logo"` // base64 encoded logo image, will be uploaded to AWS S3 and saved URL in db record
		Name    string `json:"name"`
		Intro   string `json:"intro"`
		Desc    string `json:"desc"`

		Sponsors  []string `json:"sponsors"`
		Members   []string `json:"members"`
		Proposals []string `json:"proposals"`

		Budgets []*BudgetParam `json:"budgets"`

		ContantWay   string `json:"ContantWay"`
		OfficialLink string `json:"OfficialLink"`
	}
	BudgetParam struct {
		Name        string          `json:"name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
	UpdateReq struct {
		LogoStr string `json:"logo"`
		// Name    string `json:"name"`
		// Intro   string `json:"intro"`
		Desc string `json:"desc"`

		Sponsors     []string `json:"sponsors"`
		ContantWay   string   `json:"ContantWay"`
		OfficialLink string   `json:"OfficialLink"`
	}
	DetailReply struct {
		model.Guild
		Budgets []*model.GuildBudget `json:"budgets"`
	}
	UpdateSponsorsReq struct {
		Sponsors []string `json:"sponsors"`
	}
	UpdateMembersReq struct {
		Members []string `json:"members"`
	}
	UpdateBudgetReq struct {
		Id          uint            `json:"id"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}

	GuildBudgetResp struct {
		AssetName    string `json:"asset_name"`
		TotalAmount  string `json:"total_amount"`
		UsedAmount   string `json:"used_amount"`
		RemainAmount string `json:"remain_amount"`
	}
)

type UpdateStaffsReq struct {
	Action   string   `json:"action"` // `add` or `remove`
	Sponsors []string `json:"sponsors"`
	Members  []string `json:"members"`
}

func buildGuildPermObject(guildId int) string {
	return fmt.Sprintf("%s%d", internal.ObjGuildPrefix, guildId)
}

func NormalizeWalletAddrInGuild(guild *model.Guild) *model.Guild {
	guild.Sponsors = lo.Map[string](guild.Sponsors, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})
	guild.Members = lo.Map[string](guild.Members, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})

	return guild
}

func GenerateGuildBudgetResp(budgetRcds []*model.GuildBudget) []*GuildBudgetResp {
	return lo.Map(budgetRcds, func(r *model.GuildBudget, _ int) *GuildBudgetResp {
		return &GuildBudgetResp{
			AssetName:    r.AssetName,
			TotalAmount:  r.TotalAmount.String(),
			UsedAmount:   r.UsedAmount.String(),
			RemainAmount: r.RemainAmount.String(),
		}
	})
}
