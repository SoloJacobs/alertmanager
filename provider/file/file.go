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

// Package file replays a recorded alert stream into an alert provider.
package file

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/alertmanager/eventrecorder"
	"github.com/prometheus/alertmanager/provider/mem"
	"github.com/prometheus/alertmanager/types"
)

const (
	// alertGCInterval is how often resolved alerts are dropped, matching the
	// default of the same sweep in the Alertmanager.
	alertGCInterval = 30 * time.Minute

	// resolveTimeout is how long an alert with no end time stays firing,
	// matching the Alertmanager's default.
	resolveTimeout = 5 * time.Minute
)

// Alerts is a mem.Alerts fed from a JSONL recording.
//
// A record is put into the provider at its receptionTime, the instant it was
// received. Everything a caller sees is mem's, so alerts merge, expire and
// reach subscribers exactly as they would from a live Alertmanager. A record's
// own start and end times are replayed as recorded, and the only fields filled
// in are the ones the API fills in. Records must be ordered by ascending
// receptionTime.
type Alerts struct {
	*mem.Alerts

	file   *os.File
	reader *fileReader

	quit      chan struct{}
	closeOnce sync.Once
}

// Recording is a JSONL alert recording that has been read far enough to know
// when it begins. Nothing is open or running, so a caller can put its clock at
// FirstReception before building the provider.
type Recording struct {
	// FirstReception is when the first alert of the recording was received, the
	// instant a replay has to reach before the recording begins.
	FirstReception time.Time

	path string
}

// NewRecording reads when the recording in path begins.
func NewRecording(path string) (*Recording, error) {
	first, err := firstReception(path)
	if err != nil {
		return nil, err
	}
	return &Recording{FirstReception: first, path: path}, nil
}

// NewAlerts returns a provider replaying the recording. Run starts the replay
// and Close releases the provider.
func (r *Recording) NewAlerts() (*Alerts, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}

	d, err := decompress(r.path, f)
	if err != nil {
		f.Close()
		return nil, err
	}

	m, err := mem.NewAlerts(
		context.Background(),
		alertGCInterval,
		0,
		nil,
		promslog.NewNopLogger(),
		eventrecorder.NopRecorder(),
		// Its own registry: mem skips registering its metrics when this is
		// nil, then dereferences them on every Put that has a listener.
		prometheus.NewRegistry(),
		nil,
	)
	if err != nil {
		f.Close()
		return nil, err
	}

	return &Alerts{
		Alerts: m,
		file:   f,
		reader: &fileReader{path: r.path, scanner: bufio.NewScanner(d)},
		quit:   make(chan struct{}),
	}, nil
}

// decompress wraps r when path names a gzipped recording. A recording of any
// size is mostly repeated label sets, so it is usually kept compressed.
func decompress(path string, r io.Reader) (io.Reader, error) {
	if !strings.HasSuffix(path, ".gz") {
		return r, nil
	}
	return gzip.NewReader(r)
}

// firstReception returns when the first alert of the recording in path was
// received. It reads the recording through a handle of its own, leaving the one
// the replay uses at the start of the file.
func firstReception(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	d, err := decompress(path, f)
	if err != nil {
		return time.Time{}, err
	}

	r := &fileReader{path: path, scanner: bufio.NewScanner(d)}
	if _, receptionTime, ok := r.next(); ok {
		return receptionTime, nil
	}
	return time.Time{}, fmt.Errorf("%s holds no alerts", path)
}

// Run replays the recording until it is drained or the provider is closed. The
// recording is read one record ahead of the present, since when a record is due
// is only known once it has been read.
func (a *Alerts) Run() {
	for {
		alert, receptionTime, ok := a.reader.next()
		if !ok {
			return
		}

		if wait := time.Until(receptionTime); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-t.C:
			case <-a.quit:
				t.Stop()
				return
			}
		}

		stamp(alert, time.Now())

		if err := alert.Validate(); err != nil {
			panic(fmt.Sprintf("%s: %s", a.reader.location(), err))
		}

		// Put only fails on a callback error, and this provider has none.
		if err := a.Alerts.Put(context.Background(), alert); err != nil {
			return
		}
	}
}

// stamp fills in what the API fills in when it takes an alert, so a replayed
// record reaches the provider as a posted one would. A record's receptionTime
// decides when it is replayed and nothing else, so now is read from the clock
// rather than taken from the record.
func stamp(alert *types.Alert, now time.Time) {
	alert.UpdatedAt = now

	if alert.StartsAt.IsZero() {
		if alert.EndsAt.IsZero() {
			alert.StartsAt = now
		} else {
			alert.StartsAt = alert.EndsAt
		}
	}
	if alert.EndsAt.IsZero() {
		alert.Timeout = true
		alert.EndsAt = now.Add(resolveTimeout)
	}
}

// Close stops the replay and releases the provider.
func (a *Alerts) Close() error {
	a.closeOnce.Do(func() {
		close(a.quit)
	})
	a.Alerts.Close()
	return a.file.Close()
}

// fileRecord is one line of a JSONL recording. Only lines typed as alerts are
// replayed, so a recording may carry others, such as a header.
type fileRecord struct {
	Type          string    `json:"type"`
	ReceptionTime time.Time `json:"receptionTime"`
	types.Alert
}

// fileReader reads alert records from a JSONL recording one at a time.
type fileReader struct {
	path    string
	scanner *bufio.Scanner
	line    int
}

// next returns the next alert and the instant it was received, or false at end
// of file. An unreadable record panics: the record is there and cannot be
// replayed, and the replay has nowhere to report it.
func (r *fileReader) next() (*types.Alert, time.Time, bool) {
	for r.scanner.Scan() {
		r.line++

		if len(r.scanner.Bytes()) == 0 {
			continue
		}

		var rec fileRecord
		if err := json.Unmarshal(r.scanner.Bytes(), &rec); err != nil {
			panic(fmt.Sprintf("%s: %s", r.location(), err))
		}
		if rec.Type != "alert" {
			continue
		}

		alert := rec.Alert
		return &alert, rec.ReceptionTime, true
	}

	if err := r.scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			panic(fmt.Sprintf("%s:%d: alert record exceeds %d bytes", r.path, r.line+1, bufio.MaxScanTokenSize))
		}
		panic(fmt.Sprintf("%s: %s", r.path, err))
	}

	return nil, time.Time{}, false
}

// location returns the position of the record next returned most recently, for
// reporting what is wrong with it.
func (r *fileReader) location() string {
	return fmt.Sprintf("%s:%d", r.path, r.line)
}
