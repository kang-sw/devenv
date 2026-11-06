package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// DefaultHealthTimeout bounds one /health or shutdown round trip.
const DefaultHealthTimeout = 2 * time.Second

func endpointURL(ep Endpoint, path string) string {
	return "http://127.0.0.1:" + strconv.Itoa(ep.Port) + path
}

// Health asks the daemon ep names for /health. It succeeds only when the
// answer comes from that daemon: the service name and the pid must match the
// endpoint file, so an unrelated server that later took the port is not
// mistaken for it.
func Health(ctx context.Context, client *http.Client, ep Endpoint) (HealthInfo, error) {
	if ep.Port <= 0 {
		return HealthInfo{}, fmt.Errorf("endpoint has no port")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL(ep, "/health"), nil)
	if err != nil {
		return HealthInfo{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return HealthInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HealthInfo{}, fmt.Errorf("health: status %d", resp.StatusCode)
	}
	var info HealthInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&info); err != nil {
		return HealthInfo{}, fmt.Errorf("health: %w", err)
	}
	if info.Service != healthService || info.PID != ep.PID {
		return HealthInfo{}, fmt.Errorf("health: answer is not from the recorded daemon (service %q, pid %d, want pid %d)", info.Service, info.PID, ep.PID)
	}
	return info, nil
}

// RequestShutdown asks the daemon ep names to stop, authenticated with the
// endpoint file's token.
func RequestShutdown(ctx context.Context, client *http.Client, ep Endpoint) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(ep, "/shutdown"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("shutdown: status %d", resp.StatusCode)
	}
	return nil
}

// SendNudge tells the daemon ep names to re-read one session. The caller
// treats every error as ignorable: the polling backstop catches a lost nudge.
func SendNudge(ctx context.Context, client *http.Client, ep Endpoint, body NudgeBody) error {
	if ep.Port <= 0 {
		return fmt.Errorf("endpoint has no port")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(ep, "/nudge"), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("nudge: status %d", resp.StatusCode)
	}
	return nil
}
