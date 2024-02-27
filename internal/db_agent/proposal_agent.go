package db_agent

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// proposalStateTransitRequirementsMap saves the state transition requirements for each proposal state
// The variable is a map, key is next proposal state, and the value are states can be transited to the key state
// While receiving a state transit request, using the target state to get allowed states, then append the allowed state to the where clause.
var proposalStateTransitRequirementsMap = map[model.ProposalState][]model.ProposalState{
	model.ProposalStatePendingSubmit:    {model.ProposalStatePendingSubmit},
	model.ProposalStateDraft:            {model.ProposalStatePendingSubmit},
	model.ProposalStateWithdrawn:        {model.ProposalStateDraft},
	model.ProposalStateRejected:         {model.ProposalStateDraft},
	model.ProposalStateApproved:         {model.ProposalStateDraft},
	model.ProposalStateVoting:           {model.ProposalStateDraft, model.ProposalStateApproved},
	model.ProposalStateVotePassed:       {model.ProposalStateVoting},
	model.ProposalStateVoteFailed:       {model.ProposalStateVoting},
	model.ProposalStatePendingExecution: {model.ProposalStateVotePassed},
	model.ProposalStateExecuted:         {model.ProposalStatePendingExecution},
	model.ProposalStateExecutionFailed:  {model.ProposalStatePendingExecution},
}

func UpdateProposalToStateWithStateTransitCheck(db *gorm.DB, id uint, nextState model.ProposalState) error {
	dbSeg := db.Model(&model.Proposal{}).Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"}).Where("id = ?", id)
	stateRequirements, reqFoundFlag := proposalStateTransitRequirementsMap[nextState]
	if reqFoundFlag {
		dbSeg = dbSeg.Where("state in ?", stateRequirements)
	}

	updateTx := dbSeg.Update("state", nextState)

	if updateTx.Error != nil {
		log.Error().Msgf("update proposal state error: %+v", updateTx.Error)
		return updateTx.Error
	} else if updateTx.RowsAffected == 0 {
		log.Error().Msgf("no proposal state updated: %+v", updateTx.RowsAffected)
		return gorm.ErrRecordNotFound
	} else {
		log.Debug().Msgf("update proposal %d to state: %d", id, nextState)
		return nil
	}
}
