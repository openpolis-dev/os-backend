package storage

import "github.com/casbin/casbin/v2"

// This is a temporary solution for getting enforcer object in cron jobs

var enforcer *casbin.SyncedEnforcer

func GetEnforcer() *casbin.SyncedEnforcer {
	return enforcer
}

func SetEnforcer(e *casbin.SyncedEnforcer) {
	enforcer = e
}
