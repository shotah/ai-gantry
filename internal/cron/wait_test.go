package cron_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestWaitTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in           string
		wait, nowait bool
		stripped     string
	}{
		{"Thai or pizza?\n[wait]", true, false, "Thai or pizza?"},
		{"Thai or pizza? [wait]", true, false, "Thai or pizza?"},
		{"[WAIT]\nhello", true, false, "hello"},
		{"I'll stop asking\n[nowait]", false, true, "I'll stop asking"},
		{"hello", false, false, "hello"},
		{"[silent]\n[wait]", true, false, "[silent]"},
	}
	for _, tc := range cases {
		if got := cron.HasWaitToken(tc.in); got != tc.wait {
			t.Fatalf("HasWaitToken(%q)=%v want %v", tc.in, got, tc.wait)
		}
		if got := cron.HasNoWaitToken(tc.in); got != tc.nowait {
			t.Fatalf("HasNoWaitToken(%q)=%v want %v", tc.in, got, tc.nowait)
		}
		if got := cron.StripWaitTokens(tc.in); got != tc.stripped {
			t.Fatalf("StripWaitTokens(%q)=%q want %q", tc.in, got, tc.stripped)
		}
	}
	if !cron.IsFollowUpTurn(cron.FollowUpPrefix(0, 2) + "body") {
		t.Fatal("prefix should be a follow-up turn")
	}
	if cron.IsFollowUpTurn(cron.SparkPingPrefix) {
		t.Fatal("spark is not a follow-up")
	}
}

func TestWaitService_ArmAndUserClears(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	jobs, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	w := &cron.WaitService{State: sess, Jobs: jobs, TZ: "UTC"}
	delivery := cron.Delivery{SessionID: "s", UserID: "u", ChatID: "1"}

	if err := w.AfterReply(ctx, delivery, "hey", "Thai tonight?\n[wait]"); err != nil {
		t.Fatal(err)
	}
	st, err := sess.TalkState(ctx, delivery.SessionID)
	if err != nil || !st.WaitingForReply || st.WaitNudges != 0 {
		t.Fatalf("armed: %+v err=%v", st, err)
	}
	if n := countKind(t, jobs, cron.KindFollowUp); n != 1 {
		t.Fatalf("followup jobs=%d", n)
	}

	if err := w.AfterReply(ctx, delivery, cron.FollowUpPrefix(0, 2)+"x", "still there?\n[wait]"); err != nil {
		t.Fatal(err)
	}
	st, err = sess.TalkState(ctx, delivery.SessionID)
	if err != nil || st.WaitNudges != 0 {
		t.Fatalf("follow-up [wait] must not reset nudges: %+v err=%v", st, err)
	}

	if err := w.OnUserTurn(ctx, delivery.SessionID); err != nil {
		t.Fatal(err)
	}
	st, err = sess.TalkState(ctx, delivery.SessionID)
	if err != nil || st.WaitingForReply {
		t.Fatalf("user clear: %+v err=%v", st, err)
	}
	if n := countKind(t, jobs, cron.KindFollowUp); n != 0 {
		t.Fatalf("followups after user: %d", n)
	}
}

func TestWaitService_SilentDoesNotArm(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	jobs, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	w := &cron.WaitService{State: sess, Jobs: jobs, TZ: "UTC"}
	delivery := cron.Delivery{SessionID: "s"}
	if err := w.AfterReply(ctx, delivery, "hey", cron.SilentToken+"\n[wait]"); err != nil {
		t.Fatal(err)
	}
	st, err := sess.TalkState(ctx, delivery.SessionID)
	if err != nil || st.WaitingForReply {
		t.Fatalf("silent must not arm: %+v err=%v", st, err)
	}
}

func TestWaitService_NoWaitClears(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	jobs, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	w := &cron.WaitService{State: sess, Jobs: jobs, TZ: "UTC"}
	delivery := cron.Delivery{SessionID: "s", UserID: "u"}
	if err := w.AfterReply(ctx, delivery, "hey", "ask?\n[wait]"); err != nil {
		t.Fatal(err)
	}
	if err := w.AfterReply(ctx, delivery, "hey", "dropping it\n[nowait]"); err != nil {
		t.Fatal(err)
	}
	st, err := sess.TalkState(ctx, delivery.SessionID)
	if err != nil || st.WaitingForReply {
		t.Fatalf("nowait: %+v err=%v", st, err)
	}
	if n := countKind(t, jobs, cron.KindFollowUp); n != 0 {
		t.Fatalf("jobs after nowait: %d", n)
	}
}

func TestRunner_FollowUpPokeThenExhaust(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "s", UserID: "u", ChatID: "1"}
	if err := sess.ArmWait(ctx, delivery.SessionID); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Second)
	if _, err := store.ScheduleFollowUp(ctx, delivery, "UTC", past); err != nil {
		t.Fatal(err)
	}

	pusher := &memPusher{}
	var handled []string
	runner := &cron.Runner{
		Store: store,
		Handle: func(_ context.Context, msg channel.Message) (string, error) {
			handled = append(handled, msg.Text)
			return "still thinking about dinner?", nil
		},
		Pusher: pusher,
		Talk:   sess,
		Recent: sess,
	}
	runner.FireDueForTest(ctx)
	if len(handled) != 1 || !strings.Contains(handled[0], cron.FollowUpTurnMarker) {
		t.Fatalf("handled=%v", handled)
	}
	if !strings.Contains(handled[0], "nudge 1 of 2") {
		t.Fatalf("prefix=%q", handled[0])
	}
	pusher.mu.Lock()
	nPush := len(pusher.msgs)
	pusher.mu.Unlock()
	if nPush != 1 {
		t.Fatalf("pushes=%d", nPush)
	}
	st, err := sess.TalkState(ctx, delivery.SessionID)
	if err != nil || st.WaitNudges != 1 || !st.WaitingForReply {
		t.Fatalf("after first poke: %+v err=%v", st, err)
	}
	if n := countKind(t, store, cron.KindFollowUp); n != 1 {
		t.Fatalf("next followup jobs=%d", n)
	}

	jobs, err := store.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var next cron.Job
	for _, j := range jobs {
		if j.Kind == cron.KindFollowUp {
			next = j
			break
		}
	}
	if next.ID == 0 {
		t.Fatal("expected second follow-up job")
	}
	if _, err := sess.DB().ExecContext(ctx, `UPDATE cron_job SET next_run_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-time.Second).Format("2006-01-02T15:04:05.000000000Z"), next.ID); err != nil {
		t.Fatal(err)
	}
	runner.FireDueForTest(ctx)
	if len(handled) != 2 {
		t.Fatalf("second poke handled=%d", len(handled))
	}
	st, err = sess.TalkState(ctx, delivery.SessionID)
	if err != nil || st.WaitNudges != 2 || !st.WaitingForReply {
		t.Fatalf("exhausted should keep waiting: %+v err=%v", st, err)
	}
	if n := countKind(t, store, cron.KindFollowUp); n != 0 {
		t.Fatalf("no third poke, jobs=%d", n)
	}
}

func TestRunner_FollowUpSkippedWhenNotWaiting(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "s", UserID: "u", ChatID: "1"}
	past := time.Now().UTC().Add(-time.Second)
	if _, err := store.ScheduleFollowUp(ctx, delivery, "UTC", past); err != nil {
		t.Fatal(err)
	}
	called := 0
	runner := &cron.Runner{
		Store: store,
		Handle: func(context.Context, channel.Message) (string, error) {
			called++
			return "poke", nil
		},
		Pusher: &memPusher{},
		Talk:   sess,
	}
	runner.FireDueForTest(ctx)
	if called != 0 {
		t.Fatalf("must not handle when not waiting, called=%d", called)
	}
}

func TestRunner_SparkSkippedWhileWaiting(t *testing.T) {
	ctx := context.Background()
	sess, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	store, err := cron.OpenDB(sess.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	delivery := cron.Delivery{SessionID: "s", UserID: "u", ChatID: "1"}
	if err := sess.ArmWait(ctx, delivery.SessionID); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Second)
	if _, err := store.Schedule(ctx, "spark body", cron.SparkPingParsed(past, "UTC"), delivery); err != nil {
		t.Fatal(err)
	}
	called := 0
	runner := &cron.Runner{
		Store: store,
		Handle: func(context.Context, channel.Message) (string, error) {
			called++
			return "spark", nil
		},
		Pusher: &memPusher{},
		Talk:   sess,
	}
	runner.FireDueForTest(ctx)
	if called != 0 {
		t.Fatalf("spark must skip during wait campaign, called=%d", called)
	}
}

func TestAdvanceNext_FollowUpIsOnce(t *testing.T) {
	from := time.Now().UTC()
	_, _, ok, err := cron.AdvanceNext(cron.KindFollowUp, from.Format(time.RFC3339Nano), "UTC", from)
	if err != nil || ok {
		t.Fatalf("followup should not repeat: ok=%v err=%v", err, ok)
	}
}
