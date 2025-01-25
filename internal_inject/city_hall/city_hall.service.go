package cityhall_inject

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/casbin/casbin/v2"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api/project"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type CityHallService struct {
	// inject
	Db *gorm.DB

	Cfg *config.Config
}

func (s *CityHallService) GetOrCreateCityHallProject(db *gorm.DB, enforcer *casbin.SyncedEnforcer) (*model.Project, error) {
	configuredCityHallUser, err := enforcer.GetUsersForRole(internal.RoleHall)
	if err != nil {
		return nil, errors.New("get cityhall permission error detail:" + err.Error())
	}

	cityHallProject, err := model.GetOrCreateCityHallProject(db, configuredCityHallUser)
	if err != nil {
		return nil, errors.New("get cityhall record error detail:" + err.Error())
	}

	for grpName := range cityHallProject.GroupedSponsors {
		if _, found := internal.CityhallGroupNames[grpName]; !found {
			delete(cityHallProject.GroupedSponsors, grpName)
		}
	}

	return cityHallProject, err
}

func (s *CityHallService) GenerateCityHallDetailReply(cityHallProject *model.Project, budgets []*model.ProjectBudget) *CityHallDetailReply {
	return &CityHallDetailReply{
		Project: *project.NormalizeWalletAddrInProject(cityHallProject),
		Budgets: budgets,
	}
}

func (s *CityHallService) UpdateGroupedMembers(cityHallProject *model.Project, req *CityHallUpdateMemberReq, db *gorm.DB, enforcer *casbin.SyncedEnforcer) (int, error) {
	// TODO: All checking groupName is "" is workaround logic for non grouped request, will be changed to grouped version after FE updated
	if req.GroupName != "" {
		if _, found := internal.CityhallGroupNames[req.GroupName]; !found {
			return http.StatusBadRequest, fmt.Errorf("invalid group_name %s", req.GroupName)
		}
	}

	sponsorsMap := make(map[string]bool)

	// TODO: All checking groupName is "" is workaround logic for non grouped request, will be changed to grouped version after FE updated
	if req.GroupName != "" {
		if sponsors, found := cityHallProject.GroupedSponsors[req.GroupName]; found {
			for _, sponsorWallet := range sponsors {
				sponsorsMap[common.FormatUserWallet(sponsorWallet)] = true
			}
		}
	} else {
		for _, sponsorWallet := range cityHallProject.Sponsors {
			sponsorsMap[common.FormatUserWallet(sponsorWallet)] = true
		}
	}

	///////////////////////////////
	// Update grouping policy
	///////////////////////////////

	// Add member to policy group
	var addHallGroupingPolicy [][]string
	for _, memberAddr := range req.AddMember {
		sponsorsMap[common.FormatUserWallet(memberAddr)] = true
		addHallGroupingPolicy = append(addHallGroupingPolicy, []string{common.FormatUserWallet(memberAddr), internal.RoleHall})
	}

	// Add user to hall group
	if len(addHallGroupingPolicy) > 0 {
		log.Debug().Msgf("add hall group policy: %+v", addHallGroupingPolicy)
		_, err := enforcer.AddGroupingPolicies(addHallGroupingPolicy)
		if err != nil {
			log.Error().Msgf("add hall grouping policy error %+v, policy: %+v", err, addHallGroupingPolicy)
			return http.StatusInternalServerError, err
		}
	}

	// Remove member from policy group
	var removeHallGroupingPolicy [][]string
	for _, memberAddr := range req.RemoveMember {
		sponsorsMap[common.FormatUserWallet(memberAddr)] = false
		removeHallGroupingPolicy = append(removeHallGroupingPolicy, []string{common.FormatUserWallet(memberAddr), internal.RoleHall})
	}

	// Remove user from hall group
	if len(removeHallGroupingPolicy) > 0 {
		log.Debug().Msgf("remove hall group policy: %+v", removeHallGroupingPolicy)
		_, err := enforcer.RemoveGroupingPolicies(removeHallGroupingPolicy)
		if err != nil {
			log.Error().Msgf("remove hall grouping policy error %+v, policy: %+v", err, removeHallGroupingPolicy)
			return http.StatusInternalServerError, err
		}
	}

	// Save policy
	err := enforcer.SavePolicy()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Update sponsors record in DB
	// TODO: All checking groupName is "" is workaround logic for non grouped request, will be changed to grouped version after FE updated
	db.Find(&cityHallProject, cityHallProject.ID)
	if req.GroupName != "" {
		var newSponsorsList []string
		for memberAddr, confirmedSponsors := range sponsorsMap {
			if confirmedSponsors {
				newSponsorsList = append(newSponsorsList, memberAddr)
			}
		}

		if cityHallProject.GroupedSponsors == nil {
			cityHallProject.GroupedSponsors = make(map[string][]string)
		}

		cityHallProject.GroupedSponsors[req.GroupName] = newSponsorsList
	} else {
		var newSponsorsList []string
		for memberAddr, confirmedSponsors := range sponsorsMap {
			if confirmedSponsors {
				newSponsorsList = append(newSponsorsList, memberAddr)
			}
		}
		cityHallProject.Sponsors = newSponsorsList
	}

	cityHallProject.UpdateTs = model.GetCurrentUtcEpochSecond()
	if err = db.Where(&model.Project{ID: cityHallProject.ID}).Updates(cityHallProject).Error; err != nil {
		log.Error().Msgf("update cityhall record error: %+v", err)
		return http.StatusInternalServerError, err
	}

	return http.StatusOK, nil
}
