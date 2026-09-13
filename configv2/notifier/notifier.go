package notifier

import (
	"context"
	"fmt"

	"github.com/prometheus/alertmanager/configv2/library"
)

func Main(results []library.Result) {
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		if err := r.Integration.Notifier.Notify(context.Background()); err != nil {
			fmt.Println("error:", err)
		}
	}
}
