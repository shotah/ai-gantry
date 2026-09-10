package session_test

import (
	"context"
	"testing"

	"github.com/shotah/ai-gantry/internal/session"
)

func TestTalkState_ArmClearBumpAndSpeaker(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(t.TempDir(), 20, 8000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	id := "telegram:1:1"
	st, err := store.TalkState(ctx, id)
	if err != nil || st.WaitingForReply || st.LastSpeaker != "" {
		t.Fatalf("missing session: %+v err=%v", st, err)
	}

	if err := store.ArmWait(ctx, id); err != nil {
		t.Fatal(err)
	}
	st, err = store.TalkState(ctx, id)
	if err != nil || !st.WaitingForReply || st.WaitNudges != 0 || st.LastSpeaker != session.SpeakerAgent {
		t.Fatalf("armed: %+v err=%v", st, err)
	}
	if st.Footer() == "" || st.Footer() == "[conversation] last_speaker=none waiting_for_reply=false wait_nudges=0" {
		t.Fatalf("footer=%q", st.Footer())
	}

	st, err = store.BumpWaitNudge(ctx, id)
	if err != nil || st.WaitNudges != 1 || !st.WaitingForReply {
		t.Fatalf("bump: %+v err=%v", st, err)
	}

	if err := store.ClearWait(ctx, id); err != nil {
		t.Fatal(err)
	}
	st, err = store.TalkState(ctx, id)
	if err != nil || st.WaitingForReply || st.WaitNudges != 0 {
		t.Fatalf("cleared: %+v err=%v", st, err)
	}
	if st.LastSpeaker != session.SpeakerAgent {
		t.Fatalf("clear must keep last_speaker, got %q", st.LastSpeaker)
	}

	st, err = store.BumpWaitNudge(ctx, id)
	if err != nil || st.WaitNudges != 0 {
		t.Fatalf("bump while not waiting must no-op: %+v err=%v", st, err)
	}

	if err := store.Append(ctx, id,
		session.Message{Role: session.RoleUser, Content: "hey"},
		session.Message{Role: session.RoleAssistant, Content: "hi"},
	); err != nil {
		t.Fatal(err)
	}
	st, err = store.TalkState(ctx, id)
	if err != nil || st.LastSpeaker != session.SpeakerAgent {
		t.Fatalf("append pair last_speaker=%q err=%v", st.LastSpeaker, err)
	}
}

func TestTalkState_FooterEmpty(t *testing.T) {
	if (session.TalkState{}).Footer() != "" {
		t.Fatal("zero state should have no footer")
	}
}
