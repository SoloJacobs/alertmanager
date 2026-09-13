package configv2

import (
	"fmt"
	"testing"

	"github.com/prometheus/alertmanager/configv2/library"
	"github.com/prometheus/alertmanager/configv2/notifier"
)

func TestExamples(t *testing.T) {
	for i, s := range validConfigs {
		fmt.Printf("=== valid %d ===\n", i)
		notifier.Main(library.Main(s, Integrations))
	}
	for i, s := range invalidConfigs {
		fmt.Printf("=== invalid %d ===\n", i)
		notifier.Main(library.Main(s, Integrations))
	}
}

var validConfigs = []string{
	// alpha: api_url on the receiver
	`
global: {}
receivers:
  - name: r
    alpha_configs:
      - api_url: https://alpha.example.com/
`,
	// alpha: api_url_file on the receiver
	`
global: {}
receivers:
  - name: r
    alpha_configs:
      - api_url_file: /run/secrets/alpha
`,
	// alpha: api_url inherited from global
	`
global:
  alpha_api_url: https://alpha.example.com/
receivers:
  - name: r
    alpha_configs:
      - {}
`,
	// alpha: api_url_file inherited from global
	`
global:
  alpha_api_url_file: /run/secrets/alpha
receivers:
  - name: r
    alpha_configs:
      - {}
`,
	// alpha: global holds both, the receiver picks one and the other is dropped
	`
global:
  alpha_api_url: https://alpha.example.com/
  alpha_api_url_file: /run/secrets/alpha
receivers:
  - name: r
    alpha_configs:
      - api_url: https://override.example.com/
`,
	// alpha: the receiver switches to the other member of the group
	`
global:
  alpha_api_url: https://alpha.example.com/
receivers:
  - name: r
    alpha_configs:
      - api_url_file: /run/secrets/alpha
`,
	// alpha: two entries resolve independently
	`
global:
  alpha_api_url: https://alpha.example.com/
receivers:
  - name: r
    alpha_configs:
      - {}
      - api_url_file: /run/secrets/alpha
`,
	// beta: channel on the receiver
	`
global: {}
receivers:
  - name: r
    beta_configs:
      - channel: "#team"
`,
	// beta: both keys on the receiver
	`
global: {}
receivers:
  - name: r
    beta_configs:
      - channel: "#team"
        send_resolved: false
`,
	// beta: both keys inherited from global
	`
global:
  beta_channel: "#default"
  beta_send_resolved: true
receivers:
  - name: r
    beta_configs:
      - {}
`,
	// beta: send_resolved from global, channel from the receiver
	`
global:
  beta_send_resolved: true
receivers:
  - name: r
    beta_configs:
      - channel: "#team"
`,
}

var invalidConfigs = []string{
	// alpha: both members of the group on the receiver
	`
global: {}
receivers:
  - name: r
    alpha_configs:
      - api_url: https://alpha.example.com/
        api_url_file: /run/secrets/alpha
`,
	// alpha: global holds both and the receiver picks neither
	`
global:
  alpha_api_url: https://alpha.example.com/
  alpha_api_url_file: /run/secrets/alpha
receivers:
  - name: r
    alpha_configs:
      - {}
`,
	// alpha: the group is set nowhere
	`
global: {}
receivers:
  - name: r
    alpha_configs:
      - {}
`,
	// beta: channel is set nowhere
	`
global: {}
receivers:
  - name: r
    beta_configs:
      - send_resolved: true
`,
	// beta: channel is present but empty
	`
global: {}
receivers:
  - name: r
    beta_configs:
      - channel: ""
`,
	// beta: send_resolved is not a bool
	`
global: {}
receivers:
  - name: r
    beta_configs:
      - channel: "#team"
        send_resolved: maybe
`,
}
