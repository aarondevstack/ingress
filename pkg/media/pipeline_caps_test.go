// Copyright (c) 2026 aarondevstack
// SPDX-License-Identifier: Apache-2.0

package media

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/livekit/ingress/pkg/config"
	"github.com/livekit/ingress/pkg/params"
	"github.com/livekit/ingress/pkg/stats"
	"github.com/livekit/ingress/pkg/types"
	"github.com/livekit/ingress/pkg/utils"
	"github.com/livekit/protocol/livekit"
)

func TestPipelineCapsRenegotiation(t *testing.T) {
	t.Run("repeated and changed caps", func(t *testing.T) { testPipelineCaps(t, false) })
	t.Run("retry after failed initialization", func(t *testing.T) { testPipelineCaps(t, true) })
}

func testPipelineCaps(t *testing.T, failFirst bool) {
	gst.Init(nil)
	conf, err := config.NewConfig("redis:\n  address: localhost:6379\napi_key: diagnostic\napi_secret: diagnostic-only-not-a-deployment-secret\n")
	if err != nil {
		t.Fatal(err)
	}
	info := &livekit.IngressInfo{IngressId: "caps_test", InputType: livekit.IngressInput_URL_INPUT, Url: "srt://127.0.0.1:19100", RoomName: "caps_test", ParticipantIdentity: "caps_test", State: &livekit.IngressState{ResourceId: "caps_test"}}
	pp, err := params.GetParams(context.Background(), utils.NewNoopStateNotifier(), conf, info, "", "", "", "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	codec := pp.VideoEncodingOptions.VideoCodec
	if failFirst {
		pp.VideoEncodingOptions.VideoCodec = livekit.VideoCodec(999)
	}
	sink := &WebRTCSink{params: pp, outputSync: utils.NewOutputSynchronizer(), statsGatherer: stats.NewLocalMediaStatsGatherer(), eos: newEOSDispatcher(), errChan: make(chan error, 1)}
	// This regression exercises real GST output creation and caps signals without network publication.
	sink.closed.Break()
	root, err := gst.NewPipeline("caps_test")
	if err != nil {
		t.Fatal(err)
	}
	source, err := gst.NewBinFromString("videotestsrc is-live=true ! capsfilter name=input_caps caps=video/x-raw,format=I420,width=640,height=360,framerate=30/1", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = root.Add(source.Element); err != nil {
		t.Fatal(err)
	}
	pad := source.GetStaticPad("src")
	if pad == nil {
		t.Fatal("missing source ghost pad")
	}
	p := &Pipeline{Params: pp, pipeline: root, sink: sink}
	p.onOutputReady(pad, types.Video)
	if err = root.Start(); err != nil {
		t.Fatal(err)
	}
	defer root.BlockSetState(gst.StateNull)
	defer sink.eos.Fire()
	wait := func(check func() bool) {
		t.Helper()
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			if check() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("timed out waiting for caps negotiation")
	}
	if failFirst {
		wait(func() bool { return pp.CopyInfo().State.Status == livekit.IngressState_ENDPOINT_ERROR })
		if err := root.BlockSetState(gst.StateNull); err != nil {
			t.Fatal(err)
		}
		pp.VideoEncodingOptions.VideoCodec = codec
		if err := root.Start(); err != nil {
			t.Fatal(err)
		}
	}
	wait(func() bool { return pp.CopyInfo().State.Status == livekit.IngressState_ENDPOINT_PUBLISHING })
	original, err := root.GetElementByName("video output bin")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); pad.Notify("caps") }()
	}
	wg.Wait()
	assertOutput := func() {
		t.Helper()
		current, err := root.GetElementByName("video output bin")
		if err != nil || current.Unsafe() != original.Unsafe() {
			t.Fatal("caps notification replaced the existing output bin")
		}
		state := pp.CopyInfo().State
		if state.Status != livekit.IngressState_ENDPOINT_PUBLISHING {
			t.Fatalf("caps notification broke pipeline: %s: %s", state.Status, state.Error)
		}
	}
	assertOutput()
	filter, err := source.GetElementByName("input_caps")
	if err != nil {
		t.Fatal(err)
	}
	for _, caps := range []string{"video/x-raw,format=NV12,width=640,height=360,framerate=25/1", "video/x-raw,format=I420,width=1280,height=720,framerate=30/1"} {
		if err = filter.SetProperty("caps", gst.NewCapsFromString(caps)); err != nil {
			t.Fatal(err)
		}
		expected := "NV12"
		if strings.Contains(caps, "1280") {
			expected = "width=(int)1280"
		}
		wait(func() bool {
			current := pad.GetCurrentCaps()
			return current != nil && strings.Contains(current.String(), expected)
		})
		assertOutput()
	}
}
