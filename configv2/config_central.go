// config_central.go serves as entry point for loading the configuration from
// disk. Since this module handles all different types of integrations and is
// allowed to import them, it must be extremly simple. Ideally, we want this
// file empty.
package configv2

import (
	"github.com/prometheus/alertmanager/configv2/alpha"
	"github.com/prometheus/alertmanager/configv2/beta"
	"github.com/prometheus/alertmanager/configv2/jira"
	"github.com/prometheus/alertmanager/configv2/library"
	"github.com/prometheus/alertmanager/configv2/slack"
	"github.com/prometheus/alertmanager/configv2/webhook"
)

var Integrations = map[string]library.Build{
	"alpha":   library.Register(alpha.ExtractGlobals, alpha.Groups, alpha.Parse),
	"beta":    library.Register(beta.ExtractGlobals, beta.Groups, beta.Parse),
	"jira":    library.Register(jira.ExtractGlobals, jira.Groups, jira.Parse),
	"slack":   library.Register(slack.ExtractGlobals, slack.Groups, slack.Parse),
	"webhook": library.Register(webhook.ExtractGlobals, webhook.Groups, webhook.Parse),
}
