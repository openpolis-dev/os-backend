package snsinvite_inject

type GetMySnsInviteCodeReply struct {
	InviteCode string `json:"invite_code"`
}

type GetMySnsInviteRewardsReply struct {
	InviteCount  int    `json:"invite_count"`
	TotalRewards string `json:"total_rewards"`
}
