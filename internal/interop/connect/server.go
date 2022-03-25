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

package interopconnect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bufbuild/connect"
	testrpc "github.com/bufbuild/connect-crosstest/internal/gen/proto/connect/grpc/testing/testingconnect"
	testpb "github.com/bufbuild/connect-crosstest/internal/gen/proto/go/grpc/testing"
)

const NonASCIIErrMsg = "soirée 🎉" // readable non-ASCII

type testServer struct {
	testrpc.UnimplementedTestServiceHandler
}

func NewTestConnectServer() testrpc.TestServiceHandler {
	return &testServer{}
}

func serverNewPayload(t testpb.PayloadType, size int32) (*testpb.Payload, error) {
	if size < 0 {
		return nil, fmt.Errorf("requested a response with invalid length %d", size)
	}
	body := make([]byte, size)
	switch t {
	case testpb.PayloadType_COMPRESSABLE:
	default:
		return nil, fmt.Errorf("unsupported payload type: %d", t)
	}
	return &testpb.Payload{
		Type: t,
		Body: body,
	}, nil
}

func (s *testServer) EmptyCall(ctx context.Context, req *connect.Request[testpb.Empty]) (*connect.Response[testpb.Empty], error) {
	return connect.NewResponse(new(testpb.Empty)), nil
}

func (s *testServer) UnaryCall(ctx context.Context, in *connect.Request[testpb.SimpleRequest]) (*connect.Response[testpb.SimpleResponse], error) {
	if st := in.Msg.GetResponseStatus(); st != nil && st.Code != 0 {
		return nil, connect.NewError(connect.Code(st.Code), errors.New(st.Message))
	}
	pl, err := serverNewPayload(in.Msg.GetResponseType(), in.Msg.GetResponseSize())
	if err != nil {
		return nil, err
	}
	res := connect.NewResponse(&testpb.SimpleResponse{
		Payload: pl,
	})
	if initialMetadata := in.Header().Values(initialMetadataKey); len(initialMetadata) > 0 {
		res.Header().Set(initialMetadataKey, initialMetadata[0])
	}
	if trailingMetadata := in.Header().Values(trailingMetadataKey); len(trailingMetadata) > 0 {
		res.Trailer().Set(trailingMetadataKey, trailingMetadata[0])
	}
	return res, nil
}

func (s *testServer) FailUnaryCall(ctx context.Context, in *connect.Request[testpb.SimpleRequest]) (*connect.Response[testpb.SimpleResponse], error) {
	return nil, connect.NewError(connect.CodeResourceExhausted, errors.New(NonASCIIErrMsg))
}

func (s *testServer) StreamingOutputCall(ctx context.Context, args *connect.Request[testpb.StreamingOutputCallRequest], stream *connect.ServerStream[testpb.StreamingOutputCallResponse]) error {
	cs := args.Msg.GetResponseParameters()
	for _, c := range cs {
		if us := c.GetIntervalUs(); us > 0 {
			time.Sleep(time.Duration(us) * time.Microsecond)
		}
		pl, err := serverNewPayload(args.Msg.GetResponseType(), c.GetSize())
		if err != nil {
			return err
		}
		if err := stream.Send(&testpb.StreamingOutputCallResponse{
			Payload: pl,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *testServer) StreamingInputCall(ctx context.Context, stream *connect.ClientStream[testpb.StreamingInputCallRequest, testpb.StreamingInputCallResponse]) error {
	var sum int
	for {
		if !stream.Receive() {
			if err := stream.Err(); err != nil {
				return err
			}
			return stream.SendAndClose(connect.NewResponse(&testpb.StreamingInputCallResponse{
				AggregatedPayloadSize: int32(sum),
			}))
		}
		p := stream.Msg().GetPayload().GetBody()
		sum += len(p)
	}
}

func (s *testServer) FullDuplexCall(ctx context.Context, stream *connect.BidiStream[testpb.StreamingOutputCallRequest, testpb.StreamingOutputCallResponse]) error {
	if initialMetadataRaw := ctx.Value(initialMetadataKey); initialMetadataRaw != nil {
		initialMetadata := initialMetadataRaw.([]string)
		stream.ResponseHeader().Add(initialMetadataKey, initialMetadata[0])
	}
	if trailingMetadataRaw := ctx.Value(trailingMetadataKey); trailingMetadataRaw != nil {
		trailingMetadata := trailingMetadataRaw.([]string)
		stream.ResponseTrailer().Add(trailingMetadataKey, trailingMetadata[0])
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		in, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			// read done.
			return nil
		} else if err != nil {
			return err
		}
		st := in.GetResponseStatus()
		if st != nil && st.Code != 0 {
			return connect.NewError(connect.Code(st.Code), errors.New(st.Message))
		}
		cs := in.GetResponseParameters()
		for _, c := range cs {
			if us := c.GetIntervalUs(); us > 0 {
				time.Sleep(time.Duration(us) * time.Microsecond)
			}
			pl, err := serverNewPayload(in.GetResponseType(), c.GetSize())
			if err != nil {
				return err
			}
			if err := stream.Send(&testpb.StreamingOutputCallResponse{
				Payload: pl,
			}); err != nil {
				return err
			}
		}
	}
}

func (s *testServer) HalfDuplexCall(ctx context.Context, stream *connect.BidiStream[testpb.StreamingOutputCallRequest, testpb.StreamingOutputCallResponse]) error {
	var msgBuf []*testpb.StreamingOutputCallRequest
	for {
		in, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			// read done.
			break
		}
		if err != nil {
			return err
		}
		msgBuf = append(msgBuf, in)
	}
	for _, m := range msgBuf {
		cs := m.GetResponseParameters()
		for _, c := range cs {
			if us := c.GetIntervalUs(); us > 0 {
				time.Sleep(time.Duration(us) * time.Microsecond)
			}
			pl, err := serverNewPayload(m.GetResponseType(), c.GetSize())
			if err != nil {
				return err
			}
			if err := stream.Send(&testpb.StreamingOutputCallResponse{
				Payload: pl,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}