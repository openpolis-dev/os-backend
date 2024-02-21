package db_agent

import (
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
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

func UpdateProposalState(db *gorm.DB, id uint, state model.ProposalState) error {
	return db.Model(&model.Proposal{}).Where("id = ?", id).Update("state", state).Error
}
