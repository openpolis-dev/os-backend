package cityhall_inject

import (
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/model"
)

type (
	CityHallDetailReply struct {
		model.Project
		Budgets []*model.ProjectBudget `json:"budgets"`
	}
	CityHallUpdateBudgetReq struct {
		AssetType   string          `json:"asset_type"`
		AssetName   string          `json:"asset_name"`
		TotalAmount decimal.Decimal `json:"total_amount"`
	}

	CityHallUpdateMemberReq struct {
		AddMember    []string `json:"add"`
		RemoveMember []string `json:"remove"`
		GroupName    string   `json:"group_name"`
	}
)
