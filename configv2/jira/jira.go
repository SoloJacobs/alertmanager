package jira

import (
	"context"
	"fmt"

	"github.com/prometheus/alertmanager/configv2/library"
)

type config struct {
	HTTPConfig *library.HTTPConfig `yaml:"http_config"`
	APIURL     *string             `yaml:"api_url"`
	APIType    *string             `yaml:"api_type"`
	Project    *string             `yaml:"project"`
	IssueType  *string             `yaml:"issue_type"`
	Summary    *string             `yaml:"summary"`
	Labels     *[]string           `yaml:"labels"`
}

var Groups [][]string

func ExtractGlobals(g *library.Globals) config {
	var c config
	library.Get(g, "http_config", &c.HTTPConfig)
	library.Get(g, "jira_api_url", &c.APIURL)
	return c
}

type effective struct {
	httpConfig library.HTTPConfig
	apiURL     string
	apiType    string
	project    string
	issueType  string
	summary    string
	labels     []string
}

type notifier struct {
	conf effective
}

func Parse(p *library.Parser) library.Notifier {
	var e effective

	library.Or(p, "http_config", library.HTTPConfig{}, &e.httpConfig)

	library.Required(p, "api_url", &e.apiURL)
	library.Required(p, "project", &e.project)
	library.Required(p, "issue_type", &e.issueType)
	library.Enum(p, "api_type", "auto", []string{"auto", "cloud", "datacenter"}, &e.apiType)
	library.Or(p, "summary", `{{ template "jira.default.summary" . }}`, &e.summary)
	library.Or(p, "labels", nil, &e.labels)

	return &notifier{conf: e}
}

func (n *notifier) Notify(context.Context) error {
	fmt.Printf("  jira: api_url=%s type=%s project=%s issue_type=%s labels=%v\n",
		n.conf.apiURL, n.conf.apiType, n.conf.project, n.conf.issueType, n.conf.labels)
	return nil
}
