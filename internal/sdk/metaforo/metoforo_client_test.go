package metaforo

const (
	// NOTICE: use correct token, you can copy from browser
	token = "21831|uLLroQDhdvk2OWKRHTP1wPR5vZX7vu1FmffgnBks"

	groupName       = "testttt"
	groupId         = 10434
	proposalId      = "47967"
	categoryIndexId = "1"

	groupName2       = "xs12"
	groupId2         = 10462
	categoryIndexId2 = "1"
	tagId2           = 474
	tagName2         = "待审核"
	categoryId2      = 8957
	gateTokenId2     = 67
)

var (
	// test comment id
	commentId = "1964128"
	// test comment content
	commentContent = NewContentRequest{
		Insert: "comment from os-backend unit test",
	}

	// test proposal title
	proposalTitle = "proposal from os-backend unit test"
	// test proposal content
	proposeContent = NewContentRequest{
		Insert: "proposal from os-backend unit test",
	}
)
