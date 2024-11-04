package service

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type proposalTemplateService struct{}

var ProposalTemplateService proposalTemplateService

func (*proposalTemplateService) GetUsageVoteGates(db *gorm.DB, proposalTmplId uint) (voteGates []*model.ProposalVoteGate, err error) {
	err = db.Model(&model.ProposalTemplate{}).Where("id = ?", proposalTmplId).Association("VoteGates").Find(&voteGates)
	if err != nil {
		log.Error().Msgf("get vote gates error: %+v", err)
		return
	}
	return
}
