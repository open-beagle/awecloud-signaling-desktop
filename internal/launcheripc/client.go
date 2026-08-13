package launcheripc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

func NewClientFromEnv() (*Client, error) {
	endpoint := os.Getenv(EnvLauncherEndpoint)
	token := os.Getenv(EnvLauncherToken)
	if endpoint == "" || token == "" {
		return nil, errors.New("launcher IPC environment variables missing")
	}
	return NewClient(endpoint, token), nil
}

func NewClient(endpoint, token string) *Client {
	transport := &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          5,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 35 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialLocal(ctx, endpoint)
		},
	}

	return &Client{
		endpoint: endpoint,
		token:    token,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   40 * time.Second,
		},
	}
}

func (c *Client) Connect(ctx context.Context, req *ConnectRequest) (*ConnectResponseData, error) {
	req.SchemaVersion = SchemaVersion
	req.IPCVersion = IPCVersion
	var respData ConnectResponseData
	if err := c.postJSON(ctx, "/v1/session/connect", req, &respData); err != nil {
		return nil, err
	}
	return &respData, nil
}

func (c *Client) SendAppReady(ctx context.Context, version string, pid int) error {
	req := AppReadyRequest{
		SchemaVersion: SchemaVersion,
		Version:       version,
		PID:           pid,
		ReadyAt:       time.Now().UTC(),
	}
	return c.postJSON(ctx, "/v1/session/app-ready", req, nil)
}

func (c *Client) SendServerHealthy(ctx context.Context, operationID, version string) error {
	req := ServerHealthyRequest{
		SchemaVersion: SchemaVersion,
		OperationID:   operationID,
		Version:       version,
		HeartbeatAt:   time.Now().UTC(),
	}
	return c.postJSON(ctx, "/v1/session/server-healthy", req, nil)
}

func (c *Client) ApplyUpdate(ctx context.Context, req *UpdateApplyRequest) (*UpdateAccepted, error) {
	req.SchemaVersion = SchemaVersion
	var accepted UpdateAccepted
	if err := c.postJSON(ctx, "/v1/updates/apply", req, &accepted); err != nil {
		return nil, err
	}
	return &accepted, nil
}

func (c *Client) postJSON(ctx context.Context, path string, reqBody any, outData any) error {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request body failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+HostHeader+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Host", HostHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP post failed: %w", err)
	}
	defer resp.Body.Close()

	return parseCommonResponse(resp.Body, outData)
}

func parseCommonResponse(r io.Reader, outData any) error {
	var res CommonResponse
	if err := DecodeStrictJSON(r, &res); err != nil {
		return fmt.Errorf("parse IPC response failed: %w", err)
	}
	if !res.OK {
		if res.Error != nil {
			return fmt.Errorf("IPC error [%s]: %s", res.Error.Code, res.Error.Message)
		}
		return errors.New("IPC error: response not OK")
	}
	if outData != nil && res.Data != nil {
		dataBytes, err := json.Marshal(res.Data)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(dataBytes, outData); err != nil {
			return fmt.Errorf("unmarshal data failed: %w", err)
		}
	}
	return nil
}
