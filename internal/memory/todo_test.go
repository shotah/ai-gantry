package memory

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatTodo_OldestFirstWithIdsNoCue(t *testing.T) {
	if FormatTodo(nil, time.Time{}) != "" {
		t.Fatal("empty")
	}
	now := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	rows := []Entry{
		{ID: 418, Subject: "todo/passport", Content: "renew, Wed 11am", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: 420, Subject: "todo/amazon", Content: "return the box", CreatedAt: now.Add(-30 * 24 * time.Hour)},
		{ID: 412, Subject: "todo/dentist", Content: "call to book a cleaning", UpdatedAt: now.Add(-3 * 24 * time.Hour)},
	}
	got := FormatTodo(rows, now)
	want := "[todo] #420 amazon: return the box (30d ago) · #412 dentist: call to book a cleaning (3d ago) · #418 passport: renew, Wed 11am"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if strings.Contains(got, "memory_forget") {
		t.Fatalf("a task never carries a stale cue: %q", got)
	}
	// The same age on a loop does carry the cue; the two lines must not drift together.
	loop := FormatLoops([]Entry{{Subject: "waiting/x", Content: "n", CreatedAt: now.Add(-30 * 24 * time.Hour)}}, nil, now)
	if !strings.Contains(loop, "resolve or memory_forget") {
		t.Fatalf("loops cue lost: %q", loop)
	}
}

func TestFormatTodo_CapAndOverflowHint(t *testing.T) {
	many := make([]Entry, 7)
	for i := range many {
		many[i] = Entry{ID: int64(i + 1), Subject: "todo/t" + string(rune('a'+i)), Content: "n"}
	}
	got := FormatTodo(many, time.Time{})
	if strings.Count(got, " · ") != harnessHorizonMax-1 {
		t.Fatalf("cap %q", got)
	}
	if !strings.HasSuffix(got, " (+2 more — /todo)") {
		t.Fatalf("overflow %q", got)
	}
	if !strings.HasPrefix(got, "[todo] #1 ta: n · #2 tb: n") {
		t.Fatalf("ids %q", got)
	}
}

func TestTodoBoard_ShapeAndDrops(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	// 05:00Z on the 27th is still the 26th in LA.
	at := time.Date(2026, 9, 27, 5, 0, 0, 0, time.UTC)
	rows := []Entry{
		{ID: 418, Subject: "todo/passport", Content: "  renew,\n Wed 11am ", UpdatedAt: at},
		{ID: 412, Subject: "todo/dentist", Content: "call to book a cleaning", UpdatedAt: at.Add(-72 * time.Hour)},
		{ID: 1, Subject: "todo/Bad Slug", Content: "x", UpdatedAt: at},
		{ID: 2, Subject: "todo/empty", Content: "   ", UpdatedAt: at},
		{ID: 3, Subject: "todo/long", Content: strings.Repeat("y", 300), UpdatedAt: at},
	}
	board := TodoBoard(rows, la)
	if len(board) != 3 || board[0].ID != 412 || board[1].ID != 418 || board[2].ID != 3 {
		t.Fatalf("board %+v", board)
	}
	if board[1].Text != "renew, Wed 11am" || board[1].At != "2026-09-26" || board[1].Slug != "passport" {
		t.Fatalf("item %+v", board[1])
	}
	if len([]rune(board[2].Text)) != todoTextMax {
		t.Fatalf("clip %d", len([]rune(board[2].Text)))
	}
	raw, err := json.Marshal(board[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"id":412,"slug":"dentist","text":"call to book a cleaning","at":"2026-09-23"}` {
		t.Fatalf("json %s", raw)
	}
	if got := TodoBoard(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("empty board must be [] not nil: %#v", got)
	}
}

func TestTodoPriority_MarkerParse(t *testing.T) {
	for _, tc := range []struct {
		in    string
		level int
		text  string
		words string
	}{
		{"call to book a cleaning", TodoPriorityNormal, "call to book a cleaning", "call to book a cleaning"},
		{"! renew, by Oct 15", TodoPriorityHigh, "renew, by Oct 15", "! renew, by Oct 15"},
		{"!!return the box", TodoPriorityUrgent, "return the box", "!! return the box"},
		{"  !!!  pay the fine ", TodoPriorityUrgent, "pay the fine", "!! pay the fine"},
		{"!!", TodoPriorityUrgent, "", "!!"},
		{"", TodoPriorityNormal, "", ""},
	} {
		level, text := TodoPriority(tc.in)
		if level != tc.level || text != tc.text {
			t.Errorf("TodoPriority(%q) = %d %q, want %d %q", tc.in, level, text, tc.level, tc.text)
		}
		if got := TodoWords(tc.in); got != tc.words {
			t.Errorf("TodoWords(%q) = %q, want %q", tc.in, got, tc.words)
		}
	}
}

func TestFormatTodo_PriorityBeforeAge(t *testing.T) {
	now := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	rows := []Entry{
		{ID: 420, Subject: "todo/amazon", Content: "return the box", UpdatedAt: now.Add(-30 * 24 * time.Hour)},
		{ID: 430, Subject: "todo/taxes", Content: "!! file the extension", UpdatedAt: now.Add(-1 * time.Hour)},
		{ID: 412, Subject: "todo/dentist", Content: "! call to book a cleaning", UpdatedAt: now.Add(-3 * 24 * time.Hour)},
		{ID: 418, Subject: "todo/passport", Content: "!renew, Wed 11am", UpdatedAt: now.Add(-9 * 24 * time.Hour)},
	}
	got := FormatTodo(rows, now)
	want := "[todo] #430 taxes: !! file the extension · #418 passport: ! renew, Wed 11am (9d ago) · #412 dentist: ! call to book a cleaning (3d ago) · #420 amazon: return the box (30d ago)"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	// Input order is untouched.
	if rows[0].ID != 420 || rows[1].ID != 430 {
		t.Fatal("sort mutated input")
	}
}

func TestTodoBoard_PriorityFieldAndOrder(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	rows := []Entry{
		{ID: 420, Subject: "todo/amazon", Content: "return the box", UpdatedAt: at.Add(-72 * time.Hour)},
		{ID: 430, Subject: "todo/taxes", Content: "!! file the extension", UpdatedAt: at},
		{ID: 412, Subject: "todo/dentist", Content: "!  call to book a cleaning", UpdatedAt: at.Add(-24 * time.Hour)},
		{ID: 5, Subject: "todo/bare", Content: "!!", UpdatedAt: at},
	}
	board := TodoBoard(rows, time.UTC)
	if len(board) != 3 || board[0].ID != 430 || board[1].ID != 412 || board[2].ID != 420 {
		t.Fatalf("board %+v", board)
	}
	if board[0].Priority != TodoPriorityUrgent || board[1].Priority != TodoPriorityHigh || board[2].Priority != TodoPriorityNormal {
		t.Fatalf("priority %+v", board)
	}
	if board[0].Text != "file the extension" || board[1].Text != "call to book a cleaning" {
		t.Fatalf("marker must not leak into text %+v", board)
	}
	raw, err := json.Marshal(board[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"id":430,"slug":"taxes","text":"file the extension","at":"2026-10-09","priority":2}` {
		t.Fatalf("json %s", raw)
	}
	raw, _ = json.Marshal(board[2])
	if strings.Contains(string(raw), "priority") {
		t.Fatalf("normal rows keep the old wire shape: %s", raw)
	}
}

func TestMemoryStoreDescription_NamesTodo(t *testing.T) {
	var desc string
	for _, d := range ToolDefs() {
		if d.Name == ToolStore {
			desc = d.Description
		}
	}
	for _, needle := range []string{"todo/<slug>", "even in passing", "not follow/", "never ask whether to add it", "do it now", "no cron_schedule for it", "do not offer a reminder", "never memory_recall for it", "same subject rewrites", "memory_forget by the #id on [todo]", "only when they say so", "!! urgent", "! high", "lead the words"} {
		if !strings.Contains(desc, needle) {
			t.Errorf("memory_store description missing %q", needle)
		}
	}
}
