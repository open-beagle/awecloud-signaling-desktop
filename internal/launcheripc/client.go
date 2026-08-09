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
	"net/url"
	"os"
	"strconv"
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

func (c *Client) SendServerHealthy(ctx context.Context, taskID, version string) error {
	req := ServerHealthyRequest{
		SchemaVersion: SchemaVersion,
		TaskID:        taskID,
		Version:       version,
		HeartbeatAt:   time.Now().UTC(),
	}
	return c.postJSON(ctx, "/v1/session/server-healthy", req, nil)
}

func (c *Client) RequestUpdate(ctx context.Context, req *UpdateRequest) (*UpdateSnapshot, error) {
	req.SchemaVersion = SchemaVersion
	var snapshot UpdateSnapshot
	if err := c.postJSON(ctx, "/v1/updates/request", req, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (c *Client) ConfirmUpdate(ctx context.Context, operationID string) error {
	req := UpdateConfirmRequest{
		SchemaVersion: SchemaVersion,
		OperationID:   operationID,
	}
	return c.postJSON(ctx, "/v1/updates/confirm", req, nil)
}

func (c *Client) GetState(ctx context.Context) (*StateResponseData, error) {
	var respData StateResponseData
	if err := c.getJSON(ctx, "/v1/state", &respData); err != nil {
		return nil, err
	}
	return &respData, nil
}

func (c *Client) GetEvents(ctx context.Context, afterSeq int64, waitSec int) (*EventsResponseData, error) {
	query := url.Values{}
	query.Set("after_sequence", strconv.FormatInt(afterSeq, 10))
	query.Set("wait_seconds", strconv.Itoa(waitSec))
	path := "/v1/events?" + query.Encode()

	var respData EventsResponseData
	if err := c.getJSON(ctx, path, &respData); err != nil {
		return nil, err
	}
	return &respData, nil
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

func (c *Client) getJSON(ctx context.Context, path string, outData any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+HostHeader+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Host", HostHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP get failed: %w", err)
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
