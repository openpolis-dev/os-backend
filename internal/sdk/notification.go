package sdk

import (
	"context"

	"github.com/OneSignal/onesignal-go-api"
	"github.com/rs/zerolog/log"
)

type Notificator interface {
	PushTo(id string, title string, body string, data map[string]any) error
	PushGroup(group string, title string, body string, data map[string]any) error
	PushAll(title string, body string, data map[string]any) error

	SmsTo(phones []string, title string, body string) error
	EmailTo(emails []string, title string, body string) error
}

type Notification struct {
	pusher *onesignal.APIClient
	appID  string
	apiKey string
}

func NewNotificator(apiID string, apiKey string) Notificator {
	configuration := onesignal.NewConfiguration()
	onesignalNotificator := onesignal.NewAPIClient(configuration)
	return &Notification{onesignalNotificator, apiID, apiKey}
}

func (n *Notification) PushTo(id string, title string, body string, data map[string]any) error {
	notification := onesignal.NewNotification(n.appID)
	// push to all user
	//notification.SetIncludedSegments([]string{"Subscribed Users"})
	// push to a single user
	notification.SetIncludeExternalUserIds([]string{id})
	//
	notification.SetIsIos(true)
	notification.SetIsAndroid(true)
	// set title and body
	titleMap := onesignal.StringMap{}
	titleMap.SetEn(title)
	notification.Headings = *onesignal.NewNullableStringMap(&titleMap)
	bodyMap := onesignal.StringMap{}
	bodyMap.SetEn(body)
	notification.Contents = *onesignal.NewNullableStringMap(&bodyMap)
	// set data
	notification.SetData(data)

	appAuth := context.WithValue(context.Background(), onesignal.AppAuth, n.apiKey)
	resp, r, err := n.pusher.DefaultApi.CreateNotification(appAuth).Notification(*notification).Execute()
	if err != nil {
		log.Error().Msgf("Error when calling `DefaultApi.CreateNotification``: %v\n Full HTTP response: %+v\n", err, r)
		return err
	}
	// response from `CreateNotification`: CreateNotificationSuccessResponse
	log.Debug().Msgf("Response from `DefaultApi.CreateNotification`: %+v\n", resp)

	return nil
}

func (n *Notification) PushGroup(group string, title string, body string, data map[string]any) error {
	panic("implement me")
}

func (n *Notification) PushAll(title string, body string, data map[string]any) error {
	panic("implement me")
}

func (n *Notification) SmsTo(phones []string, title string, body string) error {
	panic("implement me")
}

func (n *Notification) EmailTo(emails []string, title string, body string) error {
	panic("implement me")
}
