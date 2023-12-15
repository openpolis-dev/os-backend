package metaforo

const apiBase = "https://api.metaforo.io"

var BaseHeader = map[string]string{
	"Accept":  "application/json",
	"api_key": "metaforo_website",
}

func CreateProposal(accessToken string, content string) {
	//apiPath := "/api/submit_thread"

}
func UpdateProposal(accessToken string) {

}

func ShowProposal(accessToken string) {

}
func GetVoteData(accessToken string, proposalId string) {

}
func GetComments(accessToken string, proposalId string) {

}

func Vote(accessToken string, proposalId string) {

}

func AddComment(accessToken string, proposalId string, commentId string) {

}

func GetActionList(accessToken string) {

}
