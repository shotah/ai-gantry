package watch_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/watch"
)

func TestParseItems_Shapes(t *testing.T) {
	t.Parallel()
	items, err := watch.ParseItems(`{"items":[{"id":"a","title":"One","url":"https://x"}]}`)
	if err != nil || len(items) != 1 || items[0].ID != "a" || items[0].Title != "One" {
		t.Fatalf("items=%v err=%v", items, err)
	}
	items, err = watch.ParseItems(`[{"guid":"g1","link":"https://y"}]`)
	if err != nil || len(items) != 1 || items[0].ID != "g1" || items[0].URL != "https://y" {
		t.Fatalf("array=%v err=%v", items, err)
	}
	items, err = watch.ParseItems(`{"entries":[{"id":1,"title":"n"}]}`)
	if err != nil || len(items) != 1 || items[0].ID != "1" {
		t.Fatalf("numeric id=%v err=%v", items, err)
	}
	items, err = watch.ParseItems(`{"items":[{"title":"no id"}]}`)
	if err != nil || len(items) != 0 {
		t.Fatalf("drop no-id: %v err=%v", items, err)
	}
	if _, err := watch.ParseItems(`not json`); err == nil {
		t.Fatal("expected json error")
	}
	if _, err := watch.ParseItems(`{"foo":1}`); err == nil {
		t.Fatal("expected missing items error")
	}
	items, err = watch.ParseItems("")
	if err != nil || items != nil {
		t.Fatalf("empty: %v %v", items, err)
	}
	items, err = watch.ParseItems(`{"tweets":[{"id":"t1","text":"hi"}]}`)
	if err != nil || len(items) != 1 || items[0].ID != "t1" {
		t.Fatalf("tweets=%v err=%v", items, err)
	}
	items, err = watch.ParseItems(`{"data":[{"url":"https://only-id"}]}`)
	if err != nil || len(items) != 1 || items[0].ID != "https://only-id" {
		t.Fatalf("url-as-id=%v err=%v", items, err)
	}
	if _, err := watch.ParseItems(`{"items":"nope"}`); err == nil {
		t.Fatal("expected items-not-array")
	}
}

func TestParseItems_TruncatedJSON(t *testing.T) {
	t.Parallel()
	// Host.Call appends "\n…[truncated]" mid-string — the live gantry-tim failure.
	raw := `{"items":[{"id":"a","summary":"hello` + "\n…[truncated]"
	_, err := watch.ParseItems(raw)
	if err == nil {
		t.Fatal("expected truncated JSON error")
	}
	if !strings.Contains(err.Error(), "truncated mid-JSON") {
		t.Fatalf("err=%v", err)
	}
}

func TestDiffAndMergeSeen(t *testing.T) {
	t.Parallel()
	a := watch.Item{ID: "a"}
	b := watch.Item{ID: "b"}
	c := watch.Item{ID: "c"}
	fresh := watch.DiffNew([]watch.Item{a, b}, []string{"a"})
	if len(fresh) != 1 || fresh[0].ID != "b" {
		t.Fatalf("fresh=%v", fresh)
	}
	merged := watch.MergeSeen([]string{"a", "", "a"}, []watch.Item{a, {ID: ""}, b, c})
	if strings.Join(merged, ",") != "a,b,c" {
		t.Fatalf("merged=%v", merged)
	}
	many := make([]watch.Item, watch.MaxSeenIDs+5)
	for i := range many {
		many[i] = watch.Item{ID: fmt.Sprintf("id-%d", i)}
	}
	got := watch.MergeSeen(nil, many)
	if len(got) > watch.MaxSeenIDs {
		t.Fatalf("cap failed: %d", len(got))
	}
}

func TestFormatItems(t *testing.T) {
	t.Parallel()
	s := watch.FormatItems([]watch.Item{{ID: "1", Title: "Hi", URL: "https://x", Text: "body"}})
	if !strings.Contains(s, "id=1") || !strings.Contains(s, "Hi") {
		t.Fatalf("format=%q", s)
	}
	long := strings.Repeat("x", 300)
	s = watch.FormatItems([]watch.Item{{ID: "2", Text: long}})
	if !strings.Contains(s, "…") || strings.Contains(s, long) {
		t.Fatalf("truncate failed: %q", s)
	}
}
