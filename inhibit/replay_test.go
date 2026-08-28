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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/inhibit"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/provider/file"
)

var (
	configFile    = flag.String("config.file", "", "Alertmanager configuration to inhibit with.")
	recordingFile = flag.String("recording.file", "", "JSONL alert recording to replay.")
	flushFile     = flag.String("flush.file", "", "Flush log from the dispatcher replay, replayed alongside the alerts.")
)

// TestReplay feeds a recorded alert stream to the inhibitor, and alongside it
// replays the flush log a dispatcher produced from the same recording.
//
// The two share nothing but the clock. Alerts go to the inhibitor through the
// provider, flushes go to stdout, and each is due at the instant its own record
// carries, so they interleave the way they did when the flush log was written.
//
// Nothing asks the inhibitor whether a label set is muted: in production that
// comes from the notification pipeline and the API. What runs here is the
// ingestion half, the loop that keeps the inhibition state up to date.
func TestReplay(t *testing.T) {
	if *configFile == "" || *recordingFile == "" || *flushFile == "" {
		t.Skip("-config.file, -recording.file and -flush.file are required")
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

		flushes, err := newFlushLog(*flushFile, os.Stdout, inhibitor)
		if err != nil {
			t.Fatal(err)
		}

		go inhibitor.Run()
		inhibitor.WaitForLoading()

		var replays sync.WaitGroup
		replays.Go(alerts.Run)
		replays.Go(flushes.Run)
		replays.Wait()

		// The last alerts put are still on their way to the inhibitor, which
		// has no timer to wait on: it is done once it blocks on the
		// subscription again.
		synctest.Wait()

		inhibitor.Stop()
		if err := flushes.Close(); err != nil {
			t.Error(err)
		}
		if err := alerts.Close(); err != nil {
			t.Error(err)
		}

		// Stop returns before the goroutines it cancelled do.
		synctest.Wait()
	})
}

// flushLog replays the flush log a dispatcher replay wrote. A record carries the
// instant of its flush, so it is logged at that instant, the way the provider
// puts an alert at its receptionTime.
type flushLog struct {
	path   string
	file   *os.File
	reader *bufio.Reader
	line   int
	out    io.Writer
	muter  notify.Muter
}

func newFlushLog(path string, out io.Writer, muter notify.Muter) (*flushLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &flushLog{
		path:   path,
		file:   f,
		reader: bufio.NewReader(f),
		out:    out,
		muter:  muter,
	}, nil
}

// Run logs each record of the flush log at the instant of its flush. Records
// must be ordered by ascending time; one that is already due is logged at once.
//
// A record holds every alert of its flush, so lines run long and are read
// without a size limit rather than against a cap that a large group would trip.
func (l *flushLog) Run() {
	for {
		line, err := l.reader.ReadBytes('\n')
		if len(line) > 0 {
			l.line++
			l.replay(bytes.TrimRight(line, "\n"))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				panic(fmt.Sprintf("%s: %s", l.path, err))
			}
			return
		}
	}
}

func (l *flushLog) replay(line []byte) {
	if len(line) == 0 {
		return
	}

	var rec struct {
		Time   time.Time `json:"time"`
		Alerts []struct {
			Labels model.LabelSet `json:"labels"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		panic(fmt.Sprintf("%s:%d: %s", l.path, l.line, err))
	}

	if wait := time.Until(rec.Time); wait > 0 {
		time.Sleep(wait)
	}

	// The record is passed through as it was written.
	fmt.Fprintf(l.out, "%s\n", line)

	// The pipeline asks about every alert of the flush, one at a time.
	for _, a := range rec.Alerts {
		l.muter.Mutes(context.Background(), a.Labels)
	}
}

func (l *flushLog) Close() error {
	return l.file.Close()
}
