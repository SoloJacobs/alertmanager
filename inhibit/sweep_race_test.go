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
	"context"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	amcommoncfg "github.com/prometheus/alertmanager/config/common"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
	v29 "github.com/prometheus/alertmanager/v29inhibit"
)

// sourceGCInterval is how often an inhibition rule sweeps its source cache, as
// Inhibitor.Run starts it.
const sourceGCInterval = 15 * time.Minute

// fillerAlerts is how many resolved source alerts the sweep collects alongside
// the one under test. The callback unindexes them one at a time, so this is
// what holds the window open long enough for the refire to land inside it.
const fillerAlerts = 50000

// TestSweepUnindexesRefiredSourceAlert writes an alert stream in which a source
// alert resolves, the sweep collects it along with a large batch of other
// resolved alerts, and the alert fires again while the sweep's callback is
// still working through that batch.
//
// The refire is cached and indexed before the callback reaches the alert, and
// the callback then unindexes it by fingerprint, taking the live copy with it.
// v0.29 keeps no index and rescans the cache, so it is unaffected.
func TestSweepUnindexesRefiredSourceAlert(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()

		rule := amcommoncfg.InhibitRule{
			SourceMatch: map[string]string{"s": "1"},
			TargetMatch: map[string]string{"t": "1"},
			Equal:       []string{"e"},
		}

		provider, err := mem.NewAlerts(
			context.Background(),
			30*time.Minute,
			0,
			nil,
			promslog.NewNopLogger(),
			eventrecorder.NopRecorder(),
			prometheus.NewRegistry(),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer provider.Close()

		inhibitor := inhibit.NewInhibitor(provider, []amcommoncfg.InhibitRule{rule}, promslog.NewNopLogger(), eventrecorder.NopRecorder())
		reference := v29.NewInhibitor(provider, []amcommoncfg.InhibitRule{rule}, marker.NewAlertMarker(), promslog.NewNopLogger())

		go inhibitor.Run()
		inhibitor.WaitForLoading()
		go reference.Run()
		synctest.Wait()
		defer inhibitor.Stop()
		defer reference.Stop()

		// Every alert resolves before the sweep at sourceGCInterval, so the
		// sweep collects all of them in one batch.
		put := func(a *types.Alert) {
			if err := provider.Put(context.Background(), a); err != nil {
				t.Fatalf("put %s: %s", a.Labels, err)
			}
		}
		for i := range fillerAlerts {
			put(&types.Alert{
				Alert: model.Alert{
					Labels:   model.LabelSet{"s": "1", "e": model.LabelValue(strconv.Itoa(i)), "id": "filler"},
					StartsAt: now,
					EndsAt:   now.Add(time.Minute),
				},
				UpdatedAt: now,
			})
		}
		flapping := model.LabelSet{"s": "1", "e": "flapping"}
		put(&types.Alert{
			Alert:     model.Alert{Labels: flapping, StartsAt: now, EndsAt: now.Add(time.Minute)},
			UpdatedAt: now,
		})
		synctest.Wait()

		target := model.LabelSet{"t": "1", "e": "flapping"}

		// Due at the instant the sweep is, so the refire is put while the
		// callback is working through the batch.
		time.Sleep(sourceGCInterval)
		put(&types.Alert{
			Alert:     model.Alert{Labels: flapping, StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)},
			UpdatedAt: time.Now(),
		})
		synctest.Wait()

		got := inhibitor.Mutes(context.Background(), target)
		if want := reference.Mutes(target); got != want {
			t.Errorf("Mutes(%s) = %t, v0.29 says %t", target, got, want)
		}
	})
}
