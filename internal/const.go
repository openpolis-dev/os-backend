package internal

import (
	"time"

	"github.com/samber/lo"
)

const DefaultPageSize = 10

const EventDeleteMagicWorld = "4taoist2"

var ProjectTimezone = time.FixedZone("UTF+8", int((8 * time.Hour).Seconds()))

var ApplicationUploadTemplateHeader = map[string]string{
	"zh": "接收人,增加资产,季度,事项内容,预算来源,申请人,状态",
	"en": "Receiver,Add Assets,Season,Content,Budget Source,Operator,State",
}

var ApplicationDownloadHeader = map[string]string{
	"zh": "接收人,增加资产,季度,事项内容,预算来源,申请人,状态",
	"en": "Receiver,Add Assets,Season,Content,Budget Source,Operator,State",
}

const SeedContractType = "erc721"
const SeedContractAddr = "0x30093266E34a816a53e302bE3e59a93B52792FD4"

var CityhallGroupNames = map[string]bool{"G_GOVERNANCE": true, "G_BRANDING": true, "G_TECH": true}

// Proposal related const data

const (
	ProposalDecisionApprove string = "同意"
	ProposalDecisionReject         = "反对"
	ProposalDecisionAbstain        = "弃权"

	ProposalNumeric0   = "0%"
	ProposalNumeric30  = "30%"
	ProposalNumeric50  = "50%"
	ProposalNumeric80  = "80%"
	ProposalNumeric100 = "100%"
	ProposalNumeric120 = "120%"
)

// ProposalDecisionVoteOptions is used for proposal that requires a decision.
// This type of options only defines label of the vote, and the result can be passed or failed

// ProposalDecisionVoteOptions saves default vote options for decision vote
var ProposalDecisionVoteOptions = [][]string{
	{ProposalDecisionApprove, "1"},
	{ProposalDecisionReject, "-1"},
	{ProposalDecisionAbstain, "0"},
}
var ProposalDecisionVoteOptionsMap = lo.Associate(ProposalDecisionVoteOptions, func(r []string) (string, string) {
	return r[0], r[1]
})

// ProposalNumericVoteOptions is used to get user's options about a proposal, and the default version is used to calc ratio of rewards.
// This type of options defines label and value of the vote, the value is used for automatically tasks after vote completed.
// The vote should always pass
var ProposalNumericVoteOptions = [][]string{
	{ProposalNumeric0, "0"},
	{ProposalNumeric30, "0.3"},
	{ProposalNumeric50, "0.5"},
	{ProposalNumeric80, "0.8"},
	{ProposalNumeric100, "1"},
	{ProposalNumeric120, "1.2"},
}

var ProposalNumericVoteOptionsMap = lo.Associate(ProposalNumericVoteOptions, func(r []string) (string, string) {
	return r[0], r[1]
})

// Proposal related values

const DefaultTaskRunnerCheckIntervalSecond = 10

const TaskRefreshVotingProposalVoteInfo = "RefreshVotingProposalVoteInfo"
const TaskRefreshVotingProposalVoteInfoCronExpr = "0 * * * * * *" // Launch the job every minute

const TaskCreateProject = "project/create"
const TaskCloseProject = "project/close"
const TaskCreateGuild = "guild/create"
const TaskCloseGuild = "guild/close"
const TaskRewardNewApplication = "reward/new_application"

const ClosingProjectTemplateMagicWord = "结项"
