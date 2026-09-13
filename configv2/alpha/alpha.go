package alpha

import (
	"context"
	"fmt"

	"github.com/prometheus/alertmanager/configv2/library"
)

// config is one half of the configuration, so every field is optional.
type config struct {
	APIURL     *string `yaml:"api_url"`
	APIURLFile *string `yaml:"api_url_file"`
}

var Groups = [][]string{{"api_url", "api_url_file"}}

func ExtractGlobals(g *library.Globals) config {
	var c config
	library.Get(g, "alpha_api_url", &c.APIURL)
	library.Get(g, "alpha_api_url_file", &c.APIURLFile)
	return c
}

type effective struct {
	apiURL library.Source[string]
}

type notifier struct {
	conf effective
}

func Parse(p *library.Parser) library.Notifier {
	var e effective
	library.ValueOrFile(p, "api_url", "api_url_file", &e.apiURL)
	return &notifier{conf: e}
}

func (n *notifier) Notify(context.Context) error {
	u, err := n.conf.apiURL.Get()
	if err != nil {
		return err
	}
	fmt.Printf("  alpha: api_url=%s\n", u)
	return nil
}
