package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestProxyAcceptWithoutLegacyResourceRootServesBuildOwnedPlaybook(t *testing.T) {
	host := &proxyHost{dir: t.TempDir(), logw: io.Discard}
	session, err := host.Accept(wsdashboard.ProxyHello{
		Mode: wsdashboard.ProxyModeMCP, Version: "0.50.1", Cwd: t.TempDir(), OwnerPID: os.Getpid(),
		Env: map[string]string{"WS_CACHE_HOME": t.TempDir(), "WS_CONFIG_HOME": t.TempDir(), "WS_MCP_NO_DASHBOARD": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := session.(*proxyMCPSession).server
	defer server.Close()
	var output bytes.Buffer
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"playbook.read","arguments":{"name":"lead-workflow-manual"}}}` + "\n"
	if err := server.ServeStdio(context.Background(), strings.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Error) != 0 || response.Result.IsError || len(response.Result.Content) != 1 || !strings.Contains(response.Result.Content[0].Text, "Workflow Manual") {
		t.Fatalf("build-owned playbook without legacy root: %s", output.String())
	}
}
