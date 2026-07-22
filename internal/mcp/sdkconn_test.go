package mcp

import (
	"context"
	"sync"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKConn_ListCallClose(t *testing.T) {
	ctx := context.Background()
	type in struct {
		Name string `json:"name"`
	}
	type out struct {
		Greeting string `json:"greeting"`
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mem", Version: "v1"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "greet", Description: "hi"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input in) (*mcpsdk.CallToolResult, out, error) {
		return nil, out{Greeting: "Hello " + input.Name}, nil
	})
	t1, t2 := mcpsdk.NewInMemoryTransports()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = server.Connect(ctx, t1, nil)
	}()
	t.Cleanup(func() { wg.Wait() })

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "gantry-test", Version: "v1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn := &sdkConn{session: session}

	tools, err := conn.ListTools(ctx)
	if err != nil || len(tools) != 1 || tools[0].OriginalName != "greet" {
		t.Fatalf("tools=%v err=%v", tools, err)
	}
	got, err := conn.CallTool(ctx, "greet", map[string]any{"name": "gantry"})
	if err != nil || got == "" {
		t.Fatalf("call=%q err=%v", got, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}
