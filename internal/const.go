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
const MetaforoGroupName = "testttt"
const MetaforoGroupId = 10434

var ProposalVoteOptions = []*metaforo.VoteOption{
	{"同意", 0},
	{"反对", 1},
	{"弃权", 2},
}

const DefaultVoteStartDelay = 14 * 24 * time.Hour

// const DefaultVoteDuration = 14 * 24 * time.Hour
const DefaultVoteDuration = 5 * time.Minute
const MetaforoAdminAccessToken = "21826|C16zyP8o10wY0easORsNiCa1KTxp0AZwICUAXp6W"

const AssetTypeScrId = 1
const AssetTypeScrName = "SCR"
const AssetTypeUsdtId = 2
const AssetTypeUsdtName = "USDT"

const DefaultTaskRunnerCheckIntervalSecond = 10

const TaskRefreshVotingProposalVoteInfo = "RefreshVotingProposalVoteInfo"
