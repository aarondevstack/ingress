// Copyright 2023 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOutputSynchronizerRestartForwardJump(t *testing.T) {
	os := NewOutputSynchronizer()

	// Initial start
	dur, mediaTime := os.getWaitDuration(0, false, 0)
	require.Equal(t, time.Duration(0), dur)
	require.False(t, mediaTime.IsZero())

	// Normal sample 33ms later
	dur, mediaTime2 := os.getWaitDuration(33*time.Millisecond, true, 0)
	require.LessOrEqual(t, dur, leeway)

	// Simulate wall clock time passing during source interruption
	time.Sleep(50 * time.Millisecond)

	// Forward jump after 8s restart gap: media time jumps forward
	ptsAfterRestart := 8*time.Second + 33*time.Millisecond
	dur, mediaTimeRestart := os.getWaitDuration(ptsAfterRestart, true, 0)
	require.LessOrEqual(t, dur, leeway, "forward jump after restart should reset zeroTime and not sleep for seconds")
	require.Equal(t, time.Duration(0), dur)
	require.True(t, mediaTimeRestart.After(mediaTime2))

	// Late stale sample from before restart (pts 40ms) should be recognized as late
	durStale, _ := os.getWaitDuration(40*time.Millisecond, true, 0)
	require.Less(t, durStale, -leeway, "stale sample from before restart must be marked late for dropping")
}

func TestTrackOutputSynchronizerWaitForMediaTime(t *testing.T) {
	os := NewOutputSynchronizer()
	track := os.AddTrack()

	// Initial start
	drop, mediaTime, err := track.WaitForMediaTime(0, false)
	require.NoError(t, err)
	require.False(t, drop)
	require.False(t, mediaTime.IsZero())

	// Simulate outage
	time.Sleep(50 * time.Millisecond)

	// Forward jump: should not sleep 8s, should complete immediately
	start := time.Now()
	drop, mediaTimeRestart, err := track.WaitForMediaTime(8*time.Second, false)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.False(t, drop)
	require.Less(t, elapsed, 100*time.Millisecond, "WaitForMediaTime must not block on forward jump")
	require.True(t, mediaTimeRestart.After(mediaTime))
}
