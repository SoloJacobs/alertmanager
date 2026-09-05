#!/bin/bash
set -e

CONFIG=$PWD/alertmanager-alertgen.yaml
RECORDING=$PWD/alertgen.jsonl.gz
FLUSHES=$PWD/alertgen-flushes.jsonl.gz

go test -v -count=1 ./inhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/inhibit.log 2>&1
echo "=== inhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL|^--- (PASS|FAIL)|^ok ' /tmp/inhibit.log | sed 's/.*replay_test.go:[0-9]*: //'

go test -v -count=1 ./patchedinhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/patchedinhibit.log 2>&1
echo "=== patchedinhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL|^--- (PASS|FAIL)|^ok ' /tmp/patchedinhibit.log | sed 's/.*replay_test.go:[0-9]*: //'

go test -v -count=1 ./v29inhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/v29inhibit.log 2>&1
echo "=== v29inhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL|^--- (PASS|FAIL)|^ok ' /tmp/v29inhibit.log | sed 's/.*replay_test.go:[0-9]*: //'

go test -v -count=1 ./5449updatedinhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/5449updatedinhibit.log 2>&1
echo "=== 5449updatedinhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL|^--- (PASS|FAIL)|^ok ' /tmp/5449updatedinhibit.log | sed 's/.*replay_test.go:[0-9]*: //'

go test -v -count=1 ./5542updatedinhibit -run TestReplay -config.file=$CONFIG -recording.file=$RECORDING -flush.file=$FLUSHES > /tmp/5542updatedinhibit.log 2>&1
echo "=== 5542updatedinhibit"
grep -E 'flushes:|Mutes calls|alerts muted|fully muted|FAIL|^--- (PASS|FAIL)|^ok ' /tmp/5542updatedinhibit.log | sed 's/.*replay_test.go:[0-9]*: //'
