package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/alertmanager/configv2/library"
)

type config struct {
	HTTPConfig   *library.HTTPConfig `yaml:"http_config"`
	URL          *library.Secret     `yaml:"url"`
	URLFile      *string             `yaml:"url_file"`
	MaxAlerts    *uint64             `yaml:"max_alerts"`
	Timeout      *time.Duration      `yaml:"timeout"`
	SendResolved *bool               `yaml:"send_resolved"`
	Payload      *any                `yaml:"payload"`
}

var Groups = [][]string{{"url", "url_file"}}

func ExtractGlobals(g *library.Globals) config {
	var c config
	library.Get(g, "http_config", &c.HTTPConfig)
	library.Get(g, "webhook_url", &c.URL)
	library.Get(g, "webhook_url_file", &c.URLFile)
	library.Get(g, "webhook_max_alerts", &c.MaxAlerts)
	library.Get(g, "webhook_timeout", &c.Timeout)
	library.Get(g, "webhook_send_resolved", &c.SendResolved)
	library.Get(g, "webhook_payload", &c.Payload)
	return c
}

type effective struct {
	httpConfig library.HTTPConfig
	url        library.Source[library.Secret]
	// 0 sends every alert.
	maxAlerts uint64
	// 0 imposes no timeout.
	timeout      time.Duration
	sendResolved bool
	payload      any
}

type notifier struct {
	conf effective
}

func Parse(p *library.Parser) library.Notifier {
	var e effective

	library.Or(p, "http_config", library.HTTPConfig{}, &e.httpConfig)

	library.ValueOrFile(p, "url", "url_file", &e.url)
	library.Or(p, "max_alerts", uint64(0), &e.maxAlerts)
	library.Or(p, "timeout", time.Duration(0), &e.timeout)
	library.Or(p, "send_resolved", true, &e.sendResolved)
	library.Or(p, "payload", any(nil), &e.payload)

	return &notifier{conf: e}
}

func (n *notifier) Notify(context.Context) error {
	u, err := n.conf.url.Get()
	if err != nil {
		return err
	}
	fmt.Printf("  webhook: url=%s max_alerts=%d timeout=%s send_resolved=%t payload=%v\n",
		string(u), n.conf.maxAlerts, n.conf.timeout, n.conf.sendResolved, n.conf.payload)
	fmt.Printf("  webhook: http_config=%+v\n", n.conf.httpConfig)
	return nil
}
