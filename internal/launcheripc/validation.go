package launcheripc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const MaxRequestBodySize = 1024 * 1024 // 1 MiB

func DecodeStrictJSON[T any](r io.Reader, v *T) error {
	data, err := io.ReadAll(io.LimitReader(r, MaxRequestBodySize+1))
	if err != nil {
		return fmt.Errorf("read body failed: %w", err)
	}
	if int64(len(data)) > MaxRequestBodySize {
		return errors.New("request body size exceeds 1 MiB limit")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("decode json failed: %w", err)
	}

	if decoder.More() {
		return errors.New("trailing data after json body")
	}
	return nil
}

func ReadRequestJSON[T any](r *http.Request, v *T) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	return DecodeStrictJSON(r.Body, v)
}
