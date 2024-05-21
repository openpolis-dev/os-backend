package service

import (
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type proposalService struct{}

var ProposalService proposalService

func (*proposalService) GetProposalById(db *gorm.DB, proposalId uint) (*model.Proposal, error) {
	var proposal model.Proposal
	if err := db.First(&proposal, proposalId).Error; err != nil {
		return nil, err
	}
	return &proposal, nil
}
