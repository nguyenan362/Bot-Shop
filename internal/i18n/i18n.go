package i18n

import (
	"embed"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

//go:embed *.toml
var localeFS embed.FS

// Bundle is the global i18n bundle.
var Bundle *i18n.Bundle

func init() {
	Bundle = i18n.NewBundle(language.Vietnamese)
	Bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	// Load embedded locale files
	Bundle.LoadMessageFileFS(localeFS, "vi.toml")
	Bundle.LoadMessageFileFS(localeFS, "en.toml")
}

// T translates a message ID with optional template data.
func T(lang, messageID string, templateData map[string]interface{}) string {
	localizer := i18n.NewLocalizer(Bundle, lang)
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	if err != nil {
		return messageID // fallback to message ID
	}
	return msg
}

// TSimple translates a message ID without template data.
func TSimple(lang, messageID string) string {
	return T(lang, messageID, nil)
}
