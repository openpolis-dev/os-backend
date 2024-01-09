package internal

import (
	"time"

	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
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

// TODO: Update to real seedao group
//const MetaforoGroupName = "seedao"
//const MetaforoGroupId = 4649

const MetaforoGroupName = "testttt"
const MetaforoGroupId = 10434

var ProposalVoteOptions = []*metaforo.VoteOption{
	{"同意", 0},
	{"反对", 1},
	{"弃权", 2},
}

const DefaultVoteStartDelay = 14 * 24 * time.Hour

// Proposal related const
// TODO: move some to db records

//const DefaultVoteDuration = 2 * 24 * time.Hour // Default vote time is 2 days
// const MetaforoAdminAccessToken = "22974|rLxIV3A6DaaTrXAZ2v7UvLmo6fIwzYD4VkJCoq0N" // 3191

const DefaultVoteDuration = 2 * time.Minute // Default vote time is 2 days

const DefaultTaskRunnerCheckIntervalSecond = 10

const TaskRefreshVotingProposalVoteInfo = "RefreshVotingProposalVoteInfo"
const TaskRefreshVotingProposalVoteInfoCronExpr = "0 * * * * * *" // Launch the job every minute

const TaskCreateProject = "project/create"
const TaskCloseProject = "project/close"
const TaskCreateGuild = "guild/create"
const TaskCloseGuild = "guild/close"
const TaskRewardNewApplication = "reward/new_application"

const ProposalTitlePrefixForTesting = "[BetaTest] "
