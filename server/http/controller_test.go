// Copyright 2026 The frp Authors
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

package http

import (
	"testing"
	"time"

	"github.com/fatedier/frp/pkg/proto/wire"
	"github.com/fatedier/frp/server/registry"
)

func TestBuildClientInfoRespIncludesWireProtocol(t *testing.T) {
	info := registry.ClientInfo{
		Key:              "user.client",
		User:             "user",
		RawClientID:      "client",
		RunID:            "run-id",
		Version:          "1.0.0",
		WireProtocol:     wire.ProtocolV2,
		Hostname:         "host",
		IP:               "127.0.0.1",
		FirstConnectedAt: time.Unix(1, 0),
		LastConnectedAt:  time.Unix(2, 0),
		Online:           true,
	}

	resp := buildClientInfoResp(info)
	if resp.WireProtocol != wire.ProtocolV2 {
		t.Fatalf("wire protocol mismatch, want %q got %q", wire.ProtocolV2, resp.WireProtocol)
	}
}
