package ws_test

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/shuymn/procframe"
	testv1 "github.com/shuymn/procframe/internal/gen/test/v1"
	ws "github.com/shuymn/procframe/transport/ws"
)

func TestIntegration_WSInterceptor(t *testing.T) {
	t.Parallel()

	s := ws.NewServer(
		ws.WithInterceptors(
			procframe.StreamSendInterceptorFunc(func(next procframe.StreamSendFunc) procframe.StreamSendFunc {
				return func(resp procframe.AnyResponse) error {
					msg, ok := resp.Any().(*testv1.TickResponse)
					if !ok {
						t.Fatalf("want *testv1.TickResponse, got %T", resp.Any())
					}
					msg.Label = "wrapped:" + msg.Label
					return next(resp)
				}
			}),
		),
	)
	ws.HandleServerStream(s, "/test.v1.TickService/Watch",
		func(
			_ context.Context,
			req *procframe.Request[testv1.TickRequest],
			stream procframe.ServerStream[testv1.TickResponse],
		) error {
			for i := range req.Msg.Count {
				if err := stream.Send(&procframe.Response[testv1.TickResponse]{
					Msg: &testv1.TickResponse{
						Label: req.Msg.Label,
						Seq:   i + 1,
					},
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)

	conn, ctx := startWSServer(t, s)

	payload, err := protojson.Marshal(&testv1.TickRequest{Label: "ping", Count: 2})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	sendFrame(t, ctx, conn, inboundFrame{
		ID:        "s-1",
		Procedure: "/test.v1.TickService/Watch",
		Payload:   json.RawMessage(payload),
	})

	for i := range 2 {
		out := readFrame(t, ctx, conn)
		if out.Error != nil {
			t.Fatalf("frame %d: unexpected error: %+v", i, out.Error)
		}
		var tick testv1.TickResponse
		if err := protojson.Unmarshal(out.Payload, &tick); err != nil {
			t.Fatalf("frame %d: unmarshal: %v", i, err)
		}
		if tick.Label != "wrapped:ping" {
			t.Fatalf("frame %d: want wrapped label, got %q", i, tick.Label)
		}
	}
}
