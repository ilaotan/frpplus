package framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FrpsProxyStats mirrors the fields of frps /api/proxies used by tests.
type FrpsProxyStats struct {
	Name string         `json:"name"`
	Conf map[string]any `json:"conf"`
}

// GetFrpsProxies queries the frps dashboard for all proxies.
func GetFrpsProxies(dashboardPort int) ([]FrpsProxyStats, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/proxies", dashboardPort))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/proxies returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Proxies []FrpsProxyStats `json:"proxies"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Proxies, nil
}
