// Copyright 2022 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"

	"github.com/bufbuild/connect-crosstest/cmd/client/clienttesting"
	connectpb "github.com/bufbuild/connect-crosstest/internal/gen/proto/connect/grpc/testing/testingconnect"
	interopconnect "github.com/bufbuild/connect-crosstest/internal/interop/connect"
	"github.com/bufbuild/connect-go"
	"golang.org/x/net/http2"
)

func newClientH2C() *http.Client {
	// This is wildly insecure - don't do this in production!
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(netw, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(netw, addr)
			},
		},
	}
}

func main() {
	host := flag.String("host", "", "the host name of the test server")
	port := flag.String("port", "", "the port of the test server")
	flag.Parse()
	if *host == "" || *port == "" {
		log.Fatalf("--host and --port must both be set")
	}
	client, err := connectpb.NewTestServiceClient(
		newClientH2C(),
		"http://"+net.JoinHostPort(*host, *port),
		connect.WithGRPC(),
	)
	if err != nil {
		log.Fatalf("failed to create connect client: %v", err)
	}
	t := clienttesting.NewClientTestingT()
	interopconnect.DoEmptyUnaryCall(t, client)
	interopconnect.DoLargeUnaryCall(t, client)
	interopconnect.DoClientStreaming(t, client)
	interopconnect.DoServerStreaming(t, client)
	interopconnect.DoPingPong(t, client)
	interopconnect.DoEmptyStream(t, client)
	interopconnect.DoTimeoutOnSleepingServer(t, client)
	interopconnect.DoCancelAfterBegin(t, client)
	interopconnect.DoCancelAfterFirstResponse(t, client)
	interopconnect.DoCustomMetadata(t, client)
	interopconnect.DoStatusCodeAndMessage(t, client)
	interopconnect.DoSpecialStatusMessage(t, client)
	interopconnect.DoUnimplementedService(t, client)
	interopconnect.DoFailWithNonASCIIError(t, client)
}