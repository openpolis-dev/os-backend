package service

import (
	"errors"
	"strconv"

	"github.com/rs/zerolog/log"
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

// GetAssociatedProjectByProposalId returns project that associated to the proposal passed in.
// Only proposal for creating or closing project can have associated project
// The query is made by SIP value of the proposal and project
func (*proposalService) GetAssociatedProjectByProposalId(db *gorm.DB, proposalId uint) (*model.Project, error) {
	proposal, err := ProposalService.GetProposalById(db, proposalId)
	if err != nil {
		log.Error().Msgf("get proposal error: %+v", err)
		return nil, err
	}

	var project *model.Project
	err = db.Where(&model.Project{SIP: strconv.Itoa(proposal.Sip)}).First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debug().Msgf("no project found with SIP value %d", proposal.Sip)
			return nil, nil
		} else {
			log.Error().Msgf("get project error: %+v", err)
			return nil, err
		}
	}
	return project, nil
}
