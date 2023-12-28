package proposal

import (
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

func GetMetaforoProposalByInternalId(db *gorm.DB, proposalIdStr string) (*model.Proposal, *metaforo.ProposalResponse, error) {
	proposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		return nil, nil, err
	}

	proposalDetailRecord, err := metaforo.GetProposal(proposalRcd.GetMetaforoThreadId(), internal.MetaforoGroupName)
	if err != nil {
		return nil, nil, err
	}
	return proposalRcd, proposalDetailRecord, nil
}
