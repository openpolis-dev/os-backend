package sdk

import (
	"fmt"
	"os"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
)

func InitSentry(ginApp *gin.Engine, cfg *config.Config) {
	if cfg.ExternalServices.SentryDsn != "" {
		// Configure sentry for log collecting
		runningEnv, found := os.LookupEnv("ENV_NAME")
		if !found {
			runningEnv = "local"
		}

		if err := sentry.Init(sentry.ClientOptions{
			Dsn:           cfg.ExternalServices.SentryDsn,
			EnableTracing: true,
			// Set TracesSampleRate to 1.0 to capture 100%
			// of transactions for performance monitoring.
			// We recommend adjusting this value in production,
			TracesSampleRate: 1.0,
			Debug:            runningEnv == "local" || runningEnv == "dev",
			Environment:      runningEnv,
		}); err != nil {
			log.Error().Msgf("sentry initialization failed: %+v", err)
		}

		ginApp.Use(sentrygin.New(sentrygin.Options{
			Repanic: true,
		}))

		ginApp.Use(func(ctx *gin.Context) {
			if hub := sentrygin.GetHubFromContext(ctx); hub != nil {
				hub.Scope().SetTag("project", "os-backend")
				hub.Scope().SetTag("env", runningEnv)
			}
			ctx.Next()
		})
	}
}

func LogMessageToSentry(ctx *gin.Context, msg string, tags map[string]string) {
	if hub := sentrygin.GetHubFromContext(ctx); hub != nil {
		fmt.Print("enter sentry")
		hub.WithScope(func(scope *sentry.Scope) {
			for k, v := range tags {
				scope.SetTag(k, v)
			}
		})
		hub.CaptureMessage(msg)
	}
}

func LogServerErrorToSentry(ctx *gin.Context, err error) {
	log.Error().Msgf("server error: %+v", err)
	LogMessageToSentry(ctx, err.Error(), map[string]string{"error_scope": "server"})
}

func LogForbiddenError(ctx *gin.Context, userWallet string, targetToAccess string, action string) {
	log.Warn().Msgf("user %s is forbidden to target %s with action %s", common.FormatUserWallet(userWallet), targetToAccess, action)
	LogMessageToSentry(ctx, fmt.Sprintf("user %s is forbidden to target %s with action %s", common.FormatUserWallet(userWallet), targetToAccess, action), map[string]string{"error_scope": "user", "type": "forbidden"})
}

func LogUserSideError(ctx *gin.Context, err error) {
	log.Warn().Msgf("user side error: %+v", err)
	LogMessageToSentry(ctx, err.Error(), map[string]string{"error_scope": "user"})
}
