package db_agent

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/model"
)

func GetApplicationState(applicationId uint) (string, error) {
	var err error
	agent := GetDbAgent()

	var applicationState string
	if err = agent.db.Where(&model.Application{ID: applicationId}).Pluck("state", &applicationState).Error; err != nil {
		log.Error().Msgf("Get application %d state error: %+v", applicationId, err)
		return "", err
	}

	return applicationState, nil
}
