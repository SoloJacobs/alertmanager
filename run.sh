#!/bin/bash
set -e

CONFIG=$PWD/alertmanager-alertgen.yaml
RECORDING=$PWD/alertgen.jsonl.gz
FLUSHES=$PWD/alertgen-flushes.jsonl.gz

go test -v -count=1 ./inhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/inhibit.log 2>&1
echo "=== inhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL' /tmp/inhibit.log | sed 's/.*replay_test.go:[0-9]*: //'

go test -v -count=1 ./patchedinhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/patchedinhibit.log 2>&1
echo "=== patchedinhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL' /tmp/patchedinhibit.log | sed 's/.*replay_test.go:[0-9]*: //'

go test -v -count=1 ./v29inhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/v29inhibit.log 2>&1
echo "=== v29inhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL' /tmp/v29inhibit.log | sed 's/.*replay_test.go:[0-9]*: //'
