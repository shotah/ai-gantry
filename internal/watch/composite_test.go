package watch_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
	"github.com/shotah/ai-gantry/internal/watch"
)

func TestComposite_Routes(t *testing.T) {
	sess, err := session.Open(t.TempDir(), 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := watch.OpenDB(sess.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	other := &stubTools{defs: []provider.ToolDef{{Name: "x__y"}}}
	c := watch.Composite{
		Watch: watch.Tools{Store: store},
		Other: other,
	}
	if c.ToolCount() < 4 {
		t.Fatalf("count=%d", c.ToolCount())
	}
	ctx := cron.WithDelivery(context.Background(), cron.Delivery{SessionID: "s", UserID: "u"})
	if _, err := c.Call(ctx, watch.ToolList, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(ctx, "x__y", nil); err != nil {
		t.Fatal(err)
	}
	if other.calls != 1 {
		t.Fatalf("other calls=%d", other.calls)
	}
	if _, err := c.Call(ctx, "missing", nil); err == nil {
		t.Fatal("expected missing other error when routed")
	}
	_ = c.CallStats()
	withStats := watch.Composite{Other: &stubStats{}}
	if withStats.CallStats().TotalCalls != 1 {
		t.Fatalf("CallStats=%+v", withStats.CallStats())
	}
}

func TestComposite_NoOther(t *testing.T) {
	c := watch.Composite{}
	if c.ToolCount() != 3 {
		t.Fatalf("count=%d", c.ToolCount())
	}
	if _, err := c.Call(context.Background(), "x__y", nil); err == nil {
		t.Fatal("expected no runner")
	}
}

type stubTools struct {
	defs  []provider.ToolDef
	calls int
}

func (s *stubTools) Tools() []provider.ToolDef { return s.defs }

func (s *stubTools) ToolCount() int { return len(s.defs) }

func (s *stubTools) Call(_ context.Context, name string, _ json.RawMessage) (string, error) {
	if name != "x__y" {
		return "", context.Canceled
	}
	s.calls++
	return "ok", nil
}

type stubStats struct{ stubTools }

func (stubStats) CallStats() mcp.CallStats { return mcp.CallStats{TotalCalls: 1} }
