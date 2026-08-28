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

package dispatch_test

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/alert"
	"github.com/prometheus/alertmanager/app"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/dispatch"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/marker"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/file"
	"github.com/prometheus/alertmanager/template"
)

var (
	configFile    = flag.String("config.file", "", "Alertmanager configuration to dispatch with.")
	recordingFile = flag.String("recording.file", "", "JSONL alert recording to replay.")
	outputFile    = flag.String("output.file", "", "Where to write the flush log. Defaults to stdout.")
)

// TestReplay dispatches a recorded alert stream and logs every flush it
// produces.
//
// The dispatcher and the replay both run inside a synctest bubble, so the
// group timers cost no real time: the recording is replayed as fast as it can
// be read, whatever span it covers. The bubble's clock starts decades before
// any recording, and shifting alert timestamps into it is not allowed, so
// alerts never resolve here and no flush is ever the last one for its group.
func TestReplay(t *testing.T) {
	if *configFile == "" || *recordingFile == "" {
		t.Skip("-config.file and -recording.file are required")
	}

	conf, err := config.LoadFile(*configFile)
	if err != nil {
		t.Fatal(err)
	}

	out := io.Writer(os.Stdout)
	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		out = f
	}

	w := bufio.NewWriter(out)
	defer w.Flush()

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

		tmpl, err := template.FromGlobs(conf.Templates)
		if err != nil {
			t.Fatal(err)
		}

		routes := dispatch.NewRoute(conf.Route, nil)

		dispatcher := dispatch.NewDispatcher(
			alerts,
			routes,
			&flushLog{out: w},
			marker.NewGroupMarker(),
			// The Alertmanager's timeout function without a cluster peer.
			func(d time.Duration) time.Duration {
				return max(d, notify.MinTimeout)
			},
			app.DefaultDispatchMaintenanceInterval,
			nil,
			promslog.NewNopLogger(),
			eventrecorder.NopRecorder(),
			nil,
			tmpl,
		)

		go dispatcher.Run(time.Now())
		dispatcher.WaitForLoading()

		alerts.Run()

		// Groups still holding records read last have not flushed them yet:
		// one that has never flushed is due after its group_wait, one that has
		// after its group_interval. Sleeping strictly past the longest of both
		// leaves those timers due before this goroutine wakes, so the flush is
		// logged.
		time.Sleep(longestFlushWait(routes) + time.Nanosecond)

		dispatcher.Stop()
		if err := alerts.Close(); err != nil {
			t.Error(err)
		}

		// Aggregation groups are cancelled by Stop but not waited for, so let
		// them return before the bubble ends.
		synctest.Wait()
	})
}

func longestFlushWait(routes *dispatch.Route) time.Duration {
	var longest time.Duration
	routes.Walk(func(rt *dispatch.Route) {
		longest = max(longest, rt.RouteOpts.GroupWait, rt.RouteOpts.GroupInterval)
	})
	return longest
}

// flushLog is the whole notification pipeline of this test. The dispatcher
// calls its stage once per flush, which is the only place a flush is
// observable, and treats a nil error as notified about.
type flushLog struct {
	mtx sync.Mutex
	out io.Writer
}

type flushRecord struct {
	GroupKey string        `json:"groupKey"`
	Receiver string        `json:"receiver"`
	RouteID  string        `json:"routeId"`
	FlushID  uint64        `json:"flushId"`
	Alerts   []alertRecord `json:"alerts"`
}

type alertRecord struct {
	Labels string `json:"labels"`
	Status string `json:"status"`
}

// Exec implements notify.Stage.
func (f *flushLog) Exec(ctx context.Context, l *slog.Logger, alerts ...*alert.Alert) (context.Context, []*alert.Alert, error) {
	groupKey, _ := notify.GroupKey(ctx)
	receiver, _ := notify.ReceiverName(ctx)
	routeID, _ := notify.RouteID(ctx)
	flushID, _ := notify.FlushID(ctx)

	record := flushRecord{
		GroupKey: groupKey,
		Receiver: receiver,
		RouteID:  routeID,
		FlushID:  flushID,
		Alerts:   make([]alertRecord, len(alerts)),
	}
	for i, a := range alerts {
		record.Alerts[i] = alertRecord{
			Labels: a.Labels.String(),
			Status: string(a.Status()),
		}
	}

	// Aggregation groups flush concurrently.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if err := json.NewEncoder(f.out).Encode(record); err != nil {
		l.Error("failed to write flush log", "err", err)
	}

	return ctx, alerts, nil
}
