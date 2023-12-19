package main

import "github.com/theseed-labs/os-backend/internal/sdk/metaforo"

const groupName = "testttt"

func main() {
	//metaforo.ListProposals("", &metaforo.PaginationParams{
	//	Page:            1,
	//	PerPage:         5,
	//	CategoryIndexId: 0,
	//	TagId:           0,
	//	Sort:            "",
	//	GroupName:       groupName,
	//})

	//metaforo.GetTags(groupName, "21826|C16zyP8o10wY0easORsNiCa1KTxp0AZwICUAXp6W")
	//metaforo.NewCategory(groupName, "test_category", 0, "21826|C16zyP8o10wY0easORsNiCa1KTxp0AZwICUAXp6W")
	//metaforo.GetCategories(groupName)
	metaforo.GetGroupInfo(groupName)
}
