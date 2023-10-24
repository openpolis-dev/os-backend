package sdk

import (
	"context"

	"github.com/OneSignal/onesignal-go-api"
	"github.com/rs/zerolog/log"
)

type OneSignal struct {
	client *onesignal.APIClient
	appId  string
	appKey string
}

func NewOneSignal(appId string, appKey string) Pusher {
	configuration := onesignal.NewConfiguration()
	client := onesignal.NewAPIClient(configuration)

	return &OneSignal{client: client, appId: appId, appKey: appKey}
}

func (o *OneSignal) PushToWallets(ids []string, title map[string]string, body map[string]string, payload map[string]any) error {
	notification := onesignal.NewNotification(o.appId)
	// push to all user
	//notification.SetIncludedSegments([]string{"Subscribed Users"})
	// push to a single user
	notification.SetIncludeExternalUserIds(ids)
	////
	//notification.SetIsIos(true)
	//notification.SetIsAndroid(true)
	// set title and body
	notification.Headings = *onesignal.NewNullableStringMap(o.titleAdaptor(title))
	notification.Contents = *onesignal.NewNullableStringMap(o.bodyAdaptor(body))
	// set data
	notification.SetData(payload)

	appAuth := context.WithValue(context.Background(), onesignal.AppAuth, o.appKey)
	resp, r, err := o.client.DefaultApi.CreateNotification(appAuth).Notification(*notification).Execute()
	if err != nil {
		log.Error().Msgf("Error when calling `DefaultApi.CreateNotification``: %v\n Full HTTP response: %+v\n", err, r)
		return err
	}
	// response from `CreateNotification`: CreateNotificationSuccessResponse
	log.Debug().Msgf("Response from `DefaultApi.CreateNotification`: %+v\n", resp)

	return nil
}

func (o *OneSignal) PushAll(title map[string]string, body map[string]string, payload map[string]any) error {
	notification := onesignal.NewNotification(o.appId)
	// push to all user
	notification.SetIncludedSegments([]string{"Subscribed Users"})
	// push to a single user
	//notification.SetIncludeExternalUserIds(ids)
	////
	//notification.SetIsIos(true)
	//notification.SetIsAndroid(true)
	// set title and body
	notification.Headings = *onesignal.NewNullableStringMap(o.titleAdaptor(title))
	notification.Contents = *onesignal.NewNullableStringMap(o.bodyAdaptor(body))
	// set data
	notification.SetData(payload)

	appAuth := context.WithValue(context.Background(), onesignal.AppAuth, o.appKey)
	resp, r, err := o.client.DefaultApi.CreateNotification(appAuth).Notification(*notification).Execute()
	if err != nil {
		log.Error().Msgf("Error when calling `DefaultApi.CreateNotification``: %v\n Full HTTP response: %+v\n", err, r)
		return err
	}
	// response from `CreateNotification`: CreateNotificationSuccessResponse
	log.Debug().Msgf("Response from `DefaultApi.CreateNotification`: %+v\n", resp)

	return nil
}

func (o *OneSignal) titleAdaptor(title map[string]string) *onesignal.StringMap {
	t := &onesignal.StringMap{}
	t.SetEn(title[LanguageEN])
	t.SetZhHans(title[LanguageZH])

	return t
}

func (o *OneSignal) bodyAdaptor(body map[string]string) *onesignal.StringMap {
	b := &onesignal.StringMap{}
	b.SetEn(body[LanguageEN])
	b.SetZhHans(body[LanguageZH])

	return b
}
