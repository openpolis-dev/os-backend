package internal

import "time"

const DefaultPageSize = 10

const EventDeleteMagicWorld = "4taoist2"

var ProjectTimezone = time.FixedZone("UTF+8", int((8 * time.Hour).Seconds()))

var ApplicationUploadTemplateHeader = map[string]string{
	"zh": "钱包地址,登记积分,登记Token,事项内容,备注",
	"en": "Address,Add Points,Add Token,Content,Note",
}

var ApplicationDownloadHeader = map[string]string{
	"zh": "时间,钱包地址,登记积分,登记Token,事项内容,预算来源,备注,状态,登记人,登记人地址,审核人,审核人地址",
	"en": "Time,Address,Add Points,Add Token,Content,Budget Source,Note,State,Operator,OperatorWalletAddress,Auditor,AuditorWalletAddress",
}

const SeedContractType = "erc721"
const SeedContractAddr = "0x30093266E34a816a53e302bE3e59a93B52792FD4"

var CityhallGroupNames = []string{"G_GOVERNANCE", "G_BRANDING", "G_TECH"}
