package service

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type proposalTemplateService struct{}

var ProposalTemplateService proposalTemplateService

func (*proposalTemplateService) GetUsageVoteGates(db *gorm.DB, proposalTmpl *model.ProposalTemplate) (voteGates []*model.ProposalVoteGate, err error) {
	err = db.Model(&proposalTmpl).Association("VoteGates").Find(&voteGates)
	if err != nil {
		log.Error().Msgf("get vote gates error: %+v", err)
		return
	}
	return
}
