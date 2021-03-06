// Copyright 2019 Istio Authors
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

package pilot

import (
	"path/filepath"
	"testing"
	"time"

	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/environment"
	"istio.io/istio/pkg/test/framework/components/galley"
	"istio.io/istio/pkg/test/framework/components/pilot"
	"istio.io/istio/pkg/test/util/structpath"
)

//var (
//	ist istio.Instance
//)

func TestSidecarListeners(t *testing.T) {
	// Call Requires to explicitly initialize dependencies that the test needs.
	ctx := framework.NewContext(t)
	defer ctx.Done(t)

	// TODO: applying the examples folder requires creation of many namespaces. Limit this test to the native environment
	// until the test can be reconciled.
	ctx.RequireOrSkip(t, environment.Native)

	g := galley.NewOrFail(t, ctx, galley.Config{})
	p := pilot.NewOrFail(t, ctx, pilot.Config{Galley: g})

	// Simulate proxy identity of a sidecar ...
	nodeID := &model.Proxy{
		ClusterID:   "integration-test",
		Type:        model.SidecarProxy,
		IPAddresses: []string{"10.2.0.1"},
		ID:          "app3.testns",
		DNSDomain:   "testns.cluster.local",
	}

	// ... and get listeners from Pilot for that proxy
	req := &xdsapi.DiscoveryRequest{
		Node: &xdscore.Node{
			Id: nodeID.ServiceNode(),
		},
		TypeUrl: "type.googleapis.com/envoy.api.v2.Listener",
	}
	// Start the xDS stream
	err := p.StartDiscovery(req)
	if err != nil {
		t.Fatalf("Failed to test as no resource accepted: %v", err)
	}

	// Test the empty case where no config is loaded
	err = p.WatchDiscovery(time.Second*10,
		func(response *xdsapi.DiscoveryResponse) (b bool, e error) {
			validator := structpath.ForProto(response)
			if validator.Select("{.resources[?(@.address.socketAddress.portValue==%v)]}", 15001).Check() != nil {
				return false, nil
			}
			validateListenersNoConfig(t, validator)
			return true, nil
		})
	if err != nil {
		t.Fatalf("Failed to test as no resource accepted: %v", err)
	}

	// TODO: The code below is flaky. We should re-enable this once we have explicit config loading trigger support in Galley.
	// Apply some config
	path, err := filepath.Abs("../../testdata/config")
	if err != nil {
		t.Fatalf("No such directory: %v", err)
	}
	err = g.ApplyConfigDir(nil, path)
	if err != nil {
		t.Fatalf("Error applying directory: %v", err)
	}

	// Now continue to watch on the same stream
	err = p.WatchDiscovery(time.Second*10,
		func(response *xdsapi.DiscoveryResponse) (b bool, e error) {
			validator := structpath.ForProto(response)
			if validator.Select("{.resources[?(@.address.socketAddress.portValue==27018)]}").Check() != nil {
				return false, nil
			}
			validateMongoListener(t, validator)
			return true, nil
		})
	if err != nil {
		t.Fatalf("Failed to test as no resource accepted: %v", err)
	}
}

func validateListenersNoConfig(t *testing.T, response *structpath.Instance) {
	t.Run("validate-legacy-port-3333", func(t *testing.T) {
		// Deprecated: Should be removed as no longer needed
		response.
			Select("{.resources[?(@.address.socketAddress.portValue==3333)]}").
			Equals("10.2.0.1", "{.address.socketAddress.address}").
			Equals("envoy.tcp_proxy", "{.filterChains[0].filters[*].name}").
			Equals("inbound|3333|http|mgmtCluster", "{.filterChains[0].filters[*].config.cluster}").
			Equals(false, "{.deprecatedV1.bindToPort}").
			NotExists("{.useOriginalDst}").
			CheckOrFail(t)
	})
	t.Run("validate-legacy-port-9999", func(t *testing.T) {
		// Deprecated: Should be removed as no longer needed
		response.
			Select("{.resources[?(@.address.socketAddress.portValue==9999)]}").
			Equals("10.2.0.1", "{.address.socketAddress.address}").
			Equals("envoy.tcp_proxy", "{.filterChains[0].filters[*].name}").
			Equals("inbound|9999|custom|mgmtCluster", "{.filterChains[0].filters[*].config.cluster}").
			Equals(false, "{.deprecatedV1.bindToPort}").
			NotExists("{.useOriginalDst}").
			CheckOrFail(t)
	})
	t.Run("iptables-forwarding-listener", func(t *testing.T) {
		response.
			Select("{.resources[?(@.address.socketAddress.portValue==15001)]}").
			Equals("virtual", "{.name}").
			Equals("0.0.0.0", "{.address.socketAddress.address}").
			Equals("envoy.tcp_proxy", "{.filterChains[0].filters[*].name}").
			Equals("BlackHoleCluster", "{.filterChains[0].filters[0].config.cluster}").
			Equals("BlackHoleCluster", "{.filterChains[0].filters[0].config.stat_prefix}").
			Equals(true, "{.useOriginalDst}").
			CheckOrFail(t)
	})
}

func validateMongoListener(t *testing.T, response *structpath.Instance) {
	t.Run("validate-mongo-listener", func(t *testing.T) {
		mixerListener := response.
			Select("{.resources[?(@.address.socketAddress.portValue==%v)]}", 27018)

		mixerListener.
			Equals("0.0.0.0", "{.address.socketAddress.address}").
			// Example doing a struct comparison, note the pain with oneofs....
			Equals(&xdscore.SocketAddress{
				Address: "0.0.0.0",
				PortSpecifier: &xdscore.SocketAddress_PortValue{
					PortValue: uint32(27018),
				},
			}, "{.address.socketAddress}").
			Select("{.filterChains[0].filters[0]}").
			Equals("envoy.mongo_proxy", "{.name}").
			Select("{.config}").
			Exists("{.stat_prefix}")
	})
}

// Capturing TestMain allows us to:
// - Do cleanup before exit
// - process testing specific flags
func TestMain(m *testing.M) {
	// framework.Main("sidecar_api_test", m, istio.SetupOnKube(&ist, nil))
}
