package model

import (
	"fmt"
	"strings"
)

type ApplicationType string
type AuditActionType string
type ApplicationState string

func ParseApplicationType(typeStr string) ApplicationType {
	switch strings.ToUpper(typeStr) {
	case "CLOSE_PROJECT":
		return ApplicationCloseProject
	case "NEW_REWARD":
		return ApplicationNewReward
	default:
		panic(fmt.Errorf("unknown application type %s", typeStr))
	}
}

func (t ApplicationType) ToString() string {
	return string(t)
}

const (
	ApplicationCloseProject ApplicationType = "CLOSE_PROJECT"
	ApplicationNewReward    ApplicationType = "NEW_REWARD"
)

const (
	AuditActionNew      AuditActionType = "new"
	AuditActionApprove                  = "approve"
	AuditActionReject                   = "reject"
	AuditActionProcess                  = "process"
	AuditActionComplete                 = "complete"
)

const (
	ApplicationStateOpen       ApplicationState = "open"
	ApplicationStateApproved                    = "approved"
	ApplicationStateRejected                    = "rejected"
	ApplicationStateProcessing                  = "processing"
	ApplicationStateCompleted                   = "completed"
)

// This variable saves state transit map for all application states
var applicationStateMap = map[ApplicationState]map[AuditActionType]ApplicationState{
	ApplicationStateOpen:       {AuditActionApprove: ApplicationStateApproved, AuditActionReject: ApplicationStateRejected},
	ApplicationStateApproved:   {AuditActionProcess: ApplicationStateProcessing},
	ApplicationStateRejected:   {},
	ApplicationStateProcessing: {AuditActionComplete: ApplicationStateCompleted},
	ApplicationStateCompleted:  {},
}
