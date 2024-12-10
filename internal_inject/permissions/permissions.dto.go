package permissions_inject

type GrantRoleReq struct {
	Grants []role `json:"grants"`
}

type role struct {
	Wallet string `json:"wallet"`
	Role   string `json:"role"`
}

type RevokeRoleReq struct {
	Revokes []role `json:"revokes"`
}
