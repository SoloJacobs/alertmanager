// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package inhibit_test

import (
	"flag"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/provider/file"
)

var (
	configFile    = flag.String("config.file", "", "Alertmanager configuration to inhibit with.")
	recordingFile = flag.String("recording.file", "", "JSONL alert recording to replay.")
)

// TestReplay feeds a recorded alert stream to the inhibitor.
//
// Only the provider is wired up, so nothing ever asks the inhibitor whether a
// label set is muted: in production that comes from the notification pipeline
// and the API. What runs here is the ingestion half, the loop that keeps the
// inhibition state up to date as alerts arrive.
func TestReplay(t *testing.T) {
	if *configFile == "" || *recordingFile == "" {
		t.Skip("-config.file and -recording.file are required")
	}

	conf, err := config.LoadFile(*configFile)
	if err != nil {
		t.Fatal(err)
	}

	synctest.Test(t, func(t *testing.T) {
		recording, err := file.NewRecording(*recordingFile)
		if err != nil {
			t.Fatal(err)
		}

		// Jump the bubble to the recording while nothing is running. synctest
		// advances to the nearest due timer, so a ticker started before this
		// sleep would step the clock its own interval at a time the whole way
		// there instead of leaving this sleep to jump it at once.
		time.Sleep(time.Until(recording.FirstReception))

		alerts, err := recording.NewAlerts()
		if err != nil {
			t.Fatal(err)
		}

		inhibitor := inhibit.NewInhibitor(
			alerts,
			conf.InhibitRules,
			promslog.NewNopLogger(),
			eventrecorder.NopRecorder(),
		)

		go inhibitor.Run()
		inhibitor.WaitForLoading()

		alerts.Run()

		// The last alerts put are still on their way to the inhibitor, which
		// has no timer to wait on: it is done once it blocks on the
		// subscription again.
		synctest.Wait()

		inhibitor.Stop()
		if err := alerts.Close(); err != nil {
			t.Error(err)
		}

		// Stop returns before the goroutines it cancelled do.
		synctest.Wait()
	})
}
