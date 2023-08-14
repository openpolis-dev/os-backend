package api

const DefaultPageSize = 10

const EventDeleteMagicWorld = "4taoist2"

const CityHallRoleName = "hall"

var ApplicationUploadTemplateHeader = map[string]string{
	"zh": "钱包地址,登记积分,登记Token,事项内容,备注",
	"en": "Address,Add Points,Add Token,Content,Note",
}

var ApplicationDownloadHeader = map[string]string{
	"zh": "时间,钱包地址,登记积分,登记Token,事项内容,预算来源,备注,状态,登记人,登记人地址,审核人,审核人地址",
	"en": "Time,Address,Add Points,Add Token,Content,Budget Source,Note,State,Operator,OperatorWalletAddress,Auditor,AuditorWalletAddress",
}
