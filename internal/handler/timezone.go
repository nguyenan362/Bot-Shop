package handler

// langToTimezone maps Telegram language_code to a default IANA timezone.
// This is a best-effort mapping since Telegram doesn't expose user timezone directly.
var langToTimezone = map[string]string{
	"vi":    "Asia/Ho_Chi_Minh",
	"en":    "America/New_York",
	"zh":    "Asia/Shanghai",
	"ja":    "Asia/Tokyo",
	"ko":    "Asia/Seoul",
	"th":    "Asia/Bangkok",
	"id":    "Asia/Jakarta",
	"ms":    "Asia/Kuala_Lumpur",
	"ru":    "Europe/Moscow",
	"uk":    "Europe/Kiev",
	"de":    "Europe/Berlin",
	"fr":    "Europe/Paris",
	"es":    "Europe/Madrid",
	"pt":    "America/Sao_Paulo",
	"pt-br": "America/Sao_Paulo",
	"it":    "Europe/Rome",
	"ar":    "Asia/Riyadh",
	"hi":    "Asia/Kolkata",
	"bn":    "Asia/Dhaka",
	"tr":    "Europe/Istanbul",
	"pl":    "Europe/Warsaw",
	"nl":    "Europe/Amsterdam",
	"sv":    "Europe/Stockholm",
	"da":    "Europe/Copenhagen",
	"fi":    "Europe/Helsinki",
	"nb":    "Europe/Oslo",
	"el":    "Europe/Athens",
	"ro":    "Europe/Bucharest",
	"cs":    "Europe/Prague",
	"hu":    "Europe/Budapest",
	"he":    "Asia/Jerusalem",
	"fa":    "Asia/Tehran",
	"my":    "Asia/Yangon",
	"km":    "Asia/Phnom_Penh",
	"lo":    "Asia/Vientiane",
	"fil":   "Asia/Manila",
	"tl":    "Asia/Manila",
}

const defaultTimezone = "Asia/Ho_Chi_Minh"

// detectTimezone returns the best-guess IANA timezone from Telegram's language_code.
func detectTimezone(telegramLangCode string) string {
	if tz, ok := langToTimezone[telegramLangCode]; ok {
		return tz
	}
	return defaultTimezone
}
