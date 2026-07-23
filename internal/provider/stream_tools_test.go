package provider

import "testing"

func TestStreamToolBuf_NewIDSameIndex(t *testing.T) {
	var b streamToolBuf
	a := b.accFor(0, "a1")
	mergeName(a, "ytmusic__search_tracks")
	a.args += `{"q":"x"}`

	c := b.accFor(0, "b2")
	mergeName(c, "cast__list_local_hardware")
	c.args += `{}`

	// Further index-only deltas bind to the latest id at that index (b2).
	d := b.accFor(0, "")
	d.args += " "
	if d != c {
		t.Fatal("index-only delta should follow latest id at index")
	}
	if len(b.order) != 2 || b.order[0].name != "ytmusic__search_tracks" || b.order[1].name != "cast__list_local_hardware" {
		t.Fatalf("%+v", b.order)
	}
	if a.args != `{"q":"x"}` {
		t.Fatalf("first call args corrupted: %q", a.args)
	}
}

func TestMergeName_ResendFull(t *testing.T) {
	acc := &toolAcc{}
	mergeName(acc, "ytmusic__")
	mergeName(acc, "search_tracks")
	if acc.name != "ytmusic__search_tracks" {
		t.Fatalf("fragments: %q", acc.name)
	}
	mergeName(acc, "ytmusic__search_tracks") // full resend
	if acc.name != "ytmusic__search_tracks" {
		t.Fatalf("resend doubled: %q", acc.name)
	}
}
