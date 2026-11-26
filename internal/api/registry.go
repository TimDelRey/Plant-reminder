package api

type Route struct{ Method, Path string }

var Registry = map[string]Route{
	"auth.Service.Register":             {"POST", "/api/auth/register"},
	"auth.Service.Login":                {"POST", "/api/auth/login"},
	"auth.Service.Logout":               {"POST", "/api/auth/logout"},
	"auth.Service.RequestPasswordReset": {"POST", "/api/auth/password-reset/request"},
	"auth.Service.ConfirmPasswordReset": {"POST", "/api/auth/password-reset/confirm"},
	"auth.Service.GetMe":                {"GET", "/api/me"},
	"auth.Service.UpdateCity":           {"PATCH", "/api/me"},
	"auth.Service.DeleteAccount":        {"DELETE", "/api/me"},
	"auth.Service.ChangePassword":       {"PATCH", "/api/me/password"},
	"auth.Service.ListTokens":           {"GET", "/api/tokens"},
	"auth.Service.CreateToken":          {"POST", "/api/tokens"},
	"auth.Service.RevokeToken":          {"DELETE", "/api/tokens/{id}"},

	"cities.Service.Search": {"GET", "/api/cities"},

	"plants.Service.ListPlants":        {"GET", "/api/plants"},
	"plants.Service.CreatePlant":       {"POST", "/api/plants"},
	"plants.Service.GetPlant":          {"GET", "/api/plants/{id}"},
	"plants.Service.UpdatePlant":       {"PATCH", "/api/plants/{id}"},
	"plants.Service.DeletePlant":       {"DELETE", "/api/plants/{id}"},
	"plants.Service.ListEvents":        {"GET", "/api/plants/{id}/events"},
	"plants.Service.CreateEvent":       {"POST", "/api/plants/{id}/events"},
	"plants.Service.SuggestCare":       {"GET", "/api/plants/{id}/care-suggestions"},
	"plants.Service.SearchReference":   {"GET", "/api/plant-reference/search"},
	"plants.Service.ImportCare":        {"POST", "/api/plants/{id}/events/import"},
	"plants.Service.ListUpcoming":      {"GET", "/api/events/upcoming"},
	"plants.Service.UpdateEvent":       {"PATCH", "/api/events/{id}"},
	"plants.Service.DeleteEvent":       {"DELETE", "/api/events/{id}"},
	"plants.Service.CompleteEvent":     {"POST", "/api/events/{id}/complete"},
	"plants.Service.UndoCompleteEvent": {"POST", "/api/events/{id}/undo-complete"},

	"sender.Linking.List":            {"GET", "/api/channels"},
	"sender.Linking.Connect":         {"POST", "/api/channels/{channel}/connect"},
	"sender.Linking.Disconnect":      {"POST", "/api/channels/{channel}/disconnect"},
	"sender.Linking.Status":          {"GET", "/api/channels/{channel}/status"},
	"sender.Linking.RemoveRecipient": {"DELETE", "/api/channels/{channel}/recipients/{id}"},
}

var InternalOnly = map[string]string{
	"auth.Service.Authenticate":     "проверка авторизации в middleware, не действие пользователя",
	"sender.Linking.HandleIncoming": "вызывается вебхуками платформ (/webhooks/*), они описаны в спеке с тегом internal",
	"sender.Dispatcher.*":           "внутренний домен отправки, не действие пользователя",
	"sender.OutboxWorker.*":         "фоновый воркер очереди отправки",
	"sender.DevSender.*":            "dev-заглушка отправщика",
	"sender.Message.*":              "значение-объект сообщения",
	"sender.Channel.*":              "значение-объект канала",
	"scheduler.Scheduler.*":         "фоновый планировщик",
	"plantinfo.Service.*":           "внешний справочник растений; вызывается через plants.Service.SuggestCare (GET /api/plants/{id}/care-suggestions)",
	"plantinfo.Client.*":            "клиент Perenual API, не действие пользователя",
}

var PublicPrefixes = []string{
	"/api/auth/", "/api/cities", "/api/docs", "/api/openapi.yaml", "/webhooks/", "/internal/",
}
