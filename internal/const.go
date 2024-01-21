package internal

import (
	"time"
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

// ProposalDecisionVoteOptions is used for proposal that requires a decision.
// This type of options only defines label of the vote, and the result can be passed or failed
var ProposalDecisionVoteOptions = []string{"同意", "反对", "弃权"}

// ProposalNumericVoteOptions is used to get user's options about a proposal, and the default version is used to calc ratio of rewards.
// This type of options defines label and value of the vote, the value is used for automatically tasks after vote completed.
// The vote should always pass
var ProposalNumericVoteOptions = map[string]string{
	"0%":   "0",
	"30%":  "0.3",
	"80%":  "0.8",
	"100%": "1",
	"120%": "1.2",
}

const DefaultVoteStartDelay = 14 * 24 * time.Hour

// Proposal related values

const DefaultTaskRunnerCheckIntervalSecond = 10

const TaskRefreshVotingProposalVoteInfo = "RefreshVotingProposalVoteInfo"
const TaskRefreshVotingProposalVoteInfoCronExpr = "0 * * * * * *" // Launch the job every minute

const TaskCreateProject = "project/create"
const TaskCloseProject = "project/close"
const TaskCreateGuild = "guild/create"
const TaskCloseGuild = "guild/close"
const TaskRewardNewApplication = "reward/new_application"
