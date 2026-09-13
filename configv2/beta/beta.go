package beta

import (
	"context"
	"fmt"

	"github.com/prometheus/alertmanager/configv2/library"
)

// config is one half of the configuration, so every field is optional.
type config struct {
	SendResolved *bool           `yaml:"send_resolved"`
	Channel      *library.Secret `yaml:"channel"`
}

var Groups [][]string

// effective is the configuration the notifier runs on.
type effective struct {
	sendResolved bool
	channel      library.Secret
}

type notifier struct {
	conf effective
}

func ExtractGlobals(g *library.Globals) config {
	var c config
	library.Get(g, "beta_send_resolved", &c.SendResolved)
	library.Get(g, "beta_channel", &c.Channel)
	return c
}

func Parse(p *library.Parser) library.Notifier {
	var e effective
	library.Or(p, "send_resolved", false, &e.sendResolved)
	library.Required(p, "channel", &e.channel)
	return &notifier{conf: e}
}

func (n *notifier) Notify(context.Context) error {
	fmt.Printf("  beta: channel=%s send_resolved=%t\n", string(n.conf.channel), n.conf.sendResolved)
	return nil
}
