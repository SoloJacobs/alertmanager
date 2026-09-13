package slack

import (
	"context"
	"fmt"

	"github.com/prometheus/alertmanager/configv2/library"
)

type config struct {
	HTTPConfig    *library.HTTPConfig `yaml:"http_config"`
	APIURL        *string             `yaml:"api_url"`
	APIURLFile    *string             `yaml:"api_url_file"`
	AppToken      *library.Secret     `yaml:"app_token"`
	AppTokenFile  *string             `yaml:"app_token_file"`
	AppURL        *string             `yaml:"app_url"`
	Channel       *string             `yaml:"channel"`
	Text          *string             `yaml:"text"`
	UpdateMessage *bool               `yaml:"update_message"`
	SendResolved  *bool               `yaml:"send_resolved"`
}

// All four keys are one group, so a receiver naming its own webhook does not
// inherit the global bot token.
var Groups = [][]string{{"api_url", "api_url_file", "app_token", "app_token_file"}}

func ExtractGlobals(g *library.Globals) config {
	var c config
	library.Get(g, "http_config", &c.HTTPConfig)
	library.Get(g, "slack_api_url", &c.APIURL)
	library.Get(g, "slack_api_url_file", &c.APIURLFile)
	library.Get(g, "slack_app_token", &c.AppToken)
	library.Get(g, "slack_app_token_file", &c.AppTokenFile)
	library.Get(g, "slack_app_url", &c.AppURL)
	return c
}

// A webhook and a bot token are not interchangeable, so they do not collapse
// into one url.
type auth interface {
	isAuth()
}

type webhook struct{ url library.Source[string] }

type botToken struct {
	token library.Source[library.Secret]
}

func (webhook) isAuth()  {}
func (botToken) isAuth() {}

func newWebhook(url library.Source[string]) auth {
	return webhook{url: url}
}

func newBotToken(token library.Source[library.Secret]) auth {
	return botToken{token: token}
}

type effective struct {
	httpConfig    library.HTTPConfig
	auth          auth
	appURL        string
	channel       string
	text          string
	updateMessage bool
	sendResolved  bool
}

type notifier struct {
	conf effective
}

func Parse(p *library.Parser) library.Notifier {
	var e effective

	library.Or(p, "http_config", library.HTTPConfig{}, &e.httpConfig)

	library.OneOf(p, &e.auth,
		library.Alt(p, "api_url", "api_url_file", newWebhook),
		library.Alt(p, "app_token", "app_token_file", newBotToken),
	)

	library.Or(p, "app_url", "https://slack.com/api/chat.postMessage", &e.appURL)
	library.Or(p, "channel", "", &e.channel)
	library.Or(p, "text", `{{ template "slack.default.text" . }}`, &e.text)
	library.Or(p, "update_message", false, &e.updateMessage)
	library.Or(p, "send_resolved", false, &e.sendResolved)

	// Updating a message needs chat.update, which a webhook cannot reach.
	if _, ok := e.auth.(webhook); ok && e.updateMessage {
		p.Errorf("update_message can only be used with app_token")
	}

	return &notifier{conf: e}
}

func (n *notifier) Notify(context.Context) error {
	switch a := n.conf.auth.(type) {
	case webhook:
		u, err := a.url.Get()
		if err != nil {
			return err
		}
		fmt.Printf("  slack: webhook=%s channel=%s\n", u, n.conf.channel)
	case botToken:
		t, err := a.token.Get()
		if err != nil {
			return err
		}
		fmt.Printf("  slack: app_url=%s token=%s channel=%s update_message=%t\n",
			n.conf.appURL, string(t), n.conf.channel, n.conf.updateMessage)
	}
	return nil
}
