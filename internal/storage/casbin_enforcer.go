package storage

import "github.com/casbin/casbin/v2"

// This is a temporary solution for getting enforcer object in cron jobs

var enforcer *casbin.Enforcer

func GetEnforcer() *casbin.Enforcer {
	return enforcer
}

func SetEnforcer(e *casbin.Enforcer) {
	enforcer = e
}
