package db_agent_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/db_agent"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm/clause"
)

var caseTables = []any{
	&model.Proposal{},
	&model.ProposalVoteRecord{},
	&model.ProposalVoteOptionRecord{},
}

var (
	proposalCategory = model.ProposalCategory{
		ID:   1,
		Name: "test_category",
	}

	proposalWithVoteTypeNone = model.Proposal{
		CreateTs:           model.GetCurrentUtcEpochSecond(),
		State:              int(model.ProposalStatePendingSubmit),
		Title:              "Test proposal",
		Version:            1,
		ProposalCategoryID: 1,
		Applicant:          "",
		VoteType:           model.ProposalVoteTypeNone,
	}

	proposalWithVoteTypeDecision = model.Proposal{
		CreateTs:           model.GetCurrentUtcEpochSecond(),
		State:              int(model.ProposalStatePendingSubmit),
		Title:              "Test proposal",
		Version:            1,
		ProposalCategoryID: 1,
		Applicant:          "",
		VoteType:           model.ProposalVoteTypeDecision,
	}
)

var _ = Describe("internal/db_agent/ProposalVoteAgent", func() {
	// Before each `It` execution, create the table and init project data
	BeforeEach(func() {
		_ = db.Migrator().AutoMigrate(caseTables...)

		db.Clauses(clause.OnConflict{DoNothing: true}).Create(&proposalCategory)
	})

	// After each `It` execution, drop tables
	AfterEach(func() {
		_ = db.Migrator().DropTable(caseTables...)
	})

	Describe("Create proposal vote records", func() {
		When("proposal is created first time", func() {
			BeforeEach(func() {
				var err error
				err = db.Create(&proposalWithVoteTypeNone).Error
				Expect(err).To(BeNil())

				err = db.Create(&proposalWithVoteTypeDecision).Error
				Expect(err).To(BeNil())
			})

			It("should not create vote records if vote type is none", func() {
				voteRecords, err := db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeNone.ID, proposalWithVoteTypeNone.VoteType, nil)
				Expect(err).To(BeNil())
				Expect(voteRecords).To(BeNil())
			})

			It("should create vote records if vote type is not none", func() {
				voteOpts := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar",
					},
				}
				voteRecord, err := db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts)
				Expect(err).To(BeNil())
				Expect(voteRecord).NotTo(BeNil())
				Expect(voteRecord.ProposalID).To(BeEquivalentTo(proposalWithVoteTypeDecision.ID))
				Expect(voteOpts[0].ProposalVoteRecordId).To(BeEquivalentTo(voteRecord.ID))
			})

			It("should only create one vote record for single proposal ID even be invoked multiple times", func() {
				voteOpts := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar",
					},
				}
				voteRecord, err := db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts)
				Expect(err).To(BeNil())
				Expect(voteRecord).NotTo(BeNil())

				voteOpts2 := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar2",
					},
				}
				voteRecord2, err := db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts2)

				Expect(err).To(BeNil())
				Expect(voteRecord2).NotTo(BeNil())
				Expect(voteRecord.ID).To(BeEquivalentTo(voteRecord2.ID))
			})

			It("should recreate vote opt records for same proposal ID if not submitted to metaforo", func() {
				voteOpts := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar",
					},
				}
				_, err := db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts)
				Expect(err).To(BeNil())

				var dbVoteOptRcds []model.ProposalVoteOptionRecord
				db.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalWithVoteTypeDecision.ID).Find(&dbVoteOptRcds)
				Expect(len(dbVoteOptRcds)).To(Equal(1))

				voteOpts2 := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar2",
					},
				}
				_, err = db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts2)
				Expect(err).To(BeNil())

				// verify vote opts has been recreated
				Expect(voteOpts[0].ID).NotTo(BeEquivalentTo(voteOpts2[0].ID))
				Expect(voteOpts[0].Text).NotTo(BeEquivalentTo(voteOpts2[0].Text))

				db.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalWithVoteTypeDecision.ID).Find(&dbVoteOptRcds)
				Expect(len(dbVoteOptRcds)).To(Equal(1))
			})

			It("should not update vote opt records if has submitted to metaforo", func() {
				voteOpts := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar",
						MetaforoID: 42,
					},
				}
				_, err := db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts)
				Expect(err).To(BeNil())

				var dbVoteOptRcds []model.ProposalVoteOptionRecord
				db.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalWithVoteTypeDecision.ID).Find(&dbVoteOptRcds)
				Expect(len(dbVoteOptRcds)).To(Equal(1))

				voteOpts2 := []*model.ProposalVoteOptionRecord{
					{
						ProposalId: proposalWithVoteTypeDecision.ID,
						Text:       "foobar2",
					},
				}
				_, err = db_agent.UpsertProposalVoteRecord(db, proposalWithVoteTypeDecision.ID, proposalWithVoteTypeDecision.VoteType, voteOpts2)
				Expect(err).To(BeNil())

				// verify vote opts has not changed
				Expect(voteOpts2[0].ID).To(BeEquivalentTo(0))

				db.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalWithVoteTypeDecision.ID).Find(&dbVoteOptRcds)
				Expect(len(dbVoteOptRcds)).To(Equal(1))
				Expect(dbVoteOptRcds[0].ID).To(Equal(voteOpts[0].ID))
				Expect(dbVoteOptRcds[0].Text).To(Equal("foobar"))
			})

		})
	})
})
