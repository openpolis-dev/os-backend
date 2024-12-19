package projects_inject

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
		LogoStr string `json:"logo"` // base64 encoded image string
		Name    string `json:"name"`
		Intro   string `json:"intro"`
		Desc    string `json:"desc"`

		Sponsors  []string `json:"sponsors"`
		Members   []string `json:"members"`
		Proposals []string `json:"proposals"`

		ScrBudget  decimal.Decimal `json:"scr_budget"`
		UsdcBudget decimal.Decimal `json:"usdc_budget"`

		SIP          string `json:"SIP"`
		Category     string `json:"Category"`
		ApprovalLink string `json:"ApprovalLink"`
		OverLink     string `json:"OverLink"`
		Deliverable  string `json:"Deliverable"`
		PlanTime     string `json:"PlanTime"`
		ContantWay   string `json:"ContantWay"`
		OfficialLink string `json:"OfficialLink"`
	}

	ProjectBudgetResp struct {
		AssetName    string `json:"asset_name"`
		TotalAmount  string `json:"total_amount"`
		UsedAmount   string `json:"used_amount"`
		RemainAmount string `json:"remain_amount"`

		AdvanceRatio        string `json:"advance_ratio"`
		TotalAdvanceAmount  string `json:"total_advance_amount"`
		UsedAdvanceAmount   string `json:"used_advance_amount"`
		RemainAdvanceAmount string `json:"remain_advance_amount"`
	}

	UpdateReq struct {
		LogoStr string `json:"logo"`
		// Name    string `json:"name"`
		// Intro   string `json:"intro"`
		Desc string `json:"desc"`

		Sponsors     []string `json:"sponsors"`
		OverLink     string   `json:"OverLink"`
		ContantWay   string   `json:"ContantWay"`
		OfficialLink string   `json:"OfficialLink"`
	}
	DetailReply struct {
		model.Project
		Budgets []*ProjectBudgetResp `json:"budgets"`
	}
	UpdateBudgetReq struct {
		Id          uint            `json:"id"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}
)

type UpdateStaffsReq struct {
	Action   string   `json:"action"` // `add` or `remove`
	Sponsors []string `json:"sponsors"`
	Members  []string `json:"members"`
}

func buildProjectPermObject(projectId int) string {
	return fmt.Sprintf("%s%d", internal.ObjProjPrefix, projectId)
}

func NormalizeWalletAddrInProject(project *model.Project) *model.Project {
	project.Sponsors = lo.Map[string](project.Sponsors, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})
	project.Members = lo.Map[string](project.Members, func(wallet string, _ int) string {
		return common.ToFrontendWallet(wallet)
	})
	for grpName, wallets := range project.GroupedSponsors {
		project.GroupedSponsors[grpName] = lo.Map(wallets, func(wallet string, _ int) string {
			return common.ToFrontendWallet(wallet)
		})
	}

	return project
}

func GenerateProjectBudgetResp(budgetRcds []*model.ProjectBudget) []*ProjectBudgetResp {
	return lo.Map(budgetRcds, func(r *model.ProjectBudget, _ int) *ProjectBudgetResp {
		return &ProjectBudgetResp{
			AssetName:           r.AssetName,
			TotalAmount:         r.TotalAmount.String(),
			UsedAmount:          r.UsedAmount.String(),
			RemainAmount:        r.RemainAmount.String(),
			AdvanceRatio:        r.AdvanceRatio.String(),
			TotalAdvanceAmount:  r.TotalAdvanceAmount.String(),
			UsedAdvanceAmount:   r.UsedAdvanceAmount.String(),
			RemainAdvanceAmount: r.RemainAdvanceAmount.String(),
		}
	})
}
