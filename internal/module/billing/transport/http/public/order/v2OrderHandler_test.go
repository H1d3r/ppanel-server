package order

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/test/mock"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// streamService answers the event stream with a fixed outcome and records
// the request it served.
type streamService struct {
	billing.Service
	err    error
	events [][3]string
	got    billing.V2EventStreamRequest
}

func (s *streamService) V2StreamOrderEvents(_ context.Context, req billing.V2EventStreamRequest, sink billing.V2EventSink) error {
	s.got = req
	if s.err != nil {
		return s.err
	}
	for _, event := range s.events {
		if err := sink.Event(event[0], event[1], []byte(event[2])); err != nil {
			return nil
		}
	}
	return nil
}

// serveEvents runs the handler on a mock connection and returns the status
// and what was written: the streamed events, or a JSON refusal.
func serveEvents(t *testing.T, svc *streamService, lastEventID string) (int, string) {
	t.Helper()
	engine := server.Default()
	ctx := engine.NewContext()
	conn := mock.NewConn("")
	ctx.SetConn(conn)
	ctx.Request.Header.SetMethod(http.MethodGet)
	ctx.Request.SetRequestURI("/v2/public/orders/order-1/events?ticket=ticket-1&after=5")
	if lastEventID != "" {
		ctx.Request.Header.Set("Last-Event-ID", lastEventID)
	}
	ctx.Params = append(ctx.Params, param.Param{Key: "orderNo", Value: "order-1"})
	V2OrderEventsHandler(EventStreamDeps{Billing: svc})(context.Background(), ctx)
	if ctx.Response.GetHijackWriter() == nil {
		return ctx.Response.StatusCode(), string(ctx.Response.Body())
	}
	written := conn.WriterRecorder()
	streamed, err := written.ReadBinary(written.WroteLen())
	if err != nil {
		t.Fatal(err)
	}
	return ctx.Response.StatusCode(), string(streamed)
}

func TestV2OrderEventsHandlerStreamsTheFacadeEvents(t *testing.T) {
	svc := &streamService{events: [][3]string{{"", "order.snapshot", `{"order_no":"order-1"}`}, {"7", "order.payment_paid", `{}`}}}
	status, body := serveEvents(t, svc, "6")
	if status != http.StatusOK || !strings.Contains(body, "event: order.snapshot") || !strings.Contains(body, "id: 7") {
		t.Fatalf("response %d: %q", status, body)
	}
	// Last-Event-ID wins over the after parameter.
	if svc.got.OrderNo != "order-1" || svc.got.Ticket != "ticket-1" || svc.got.AfterID != 6 {
		t.Fatalf("stream request = %+v", svc.got)
	}
	serveEvents(t, svc, "")
	if svc.got.AfterID != 5 {
		t.Fatalf("after parameter = %d, want 5", svc.got.AfterID)
	}
}

func TestV2OrderEventsHandlerAnswersRefusalsWithJSON(t *testing.T) {
	for name, tt := range map[string]struct {
		err        error
		wantStatus int
		wantCode   uint32
	}{
		"too many streams": {billing.ErrTooManyEventStreams, http.StatusTooManyRequests, xerr.TooManyRequests},
		"invalid ticket":   {errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "event ticket is invalid"), http.StatusOK, xerr.InvalidAccess},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := serveEvents(t, &streamService{err: tt.err}, "")
			var response struct {
				Code uint32 `json:"code"`
			}
			if err := json.Unmarshal([]byte(body), &response); err != nil {
				t.Fatalf("body %q: %v", body, err)
			}
			if status != tt.wantStatus || response.Code != tt.wantCode {
				t.Fatalf("response %d %q, want %d with code %d", status, body, tt.wantStatus, tt.wantCode)
			}
		})
	}
}
