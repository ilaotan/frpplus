// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"errors"
	"fmt"

	"github.com/fatedier/frp/pkg/util/jsonx"
)

type DecodeOptions struct {
	DisallowUnknownFields bool
}

func decodeJSONWithOptions(b []byte, out any, options DecodeOptions) error {
	return jsonx.UnmarshalWithOptions(b, out, jsonx.DecodeOptions{
		RejectUnknownMembers: options.DisallowUnknownFields,
	})
}

func isJSONNull(b []byte) bool {
	return len(b) == 0 || string(b) == "null"
}

type typedEnvelope struct {
	Type string `json:"type"`
}

func DecodeProxyConfigurerJSON(b []byte, options DecodeOptions) (ProxyConfigurer, error) {
	if isJSONNull(b) {
		return nil, errors.New("type is required")
	}

	var env typedEnvelope
	if err := jsonx.Unmarshal(b, &env); err != nil {
		return nil, err
	}

	configurer := NewProxyConfigurerByType(ProxyType(env.Type))
	if configurer == nil {
		return nil, fmt.Errorf("unknown proxy type: %s", env.Type)
	}
	if err := decodeJSONWithOptions(b, configurer, options); err != nil {
		return nil, fmt.Errorf("unmarshal ProxyConfig error: %v", err)
	}

	return configurer, nil
}

func DecodeClientConfigJSON(b []byte, options DecodeOptions) (ClientConfig, error) {
	type rawClientConfig struct {
		ClientCommonConfig
		Proxies []jsonx.RawMessage `json:"proxies,omitempty"`
	}

	raw := rawClientConfig{}
	if err := decodeJSONWithOptions(b, &raw, options); err != nil {
		return ClientConfig{}, err
	}

	cfg := ClientConfig{
		ClientCommonConfig: raw.ClientCommonConfig,
		Proxies:            make([]TypedProxyConfig, 0, len(raw.Proxies)),
	}

	for i, proxyData := range raw.Proxies {
		proxyCfg, err := DecodeProxyConfigurerJSON(proxyData, options)
		if err != nil {
			return ClientConfig{}, fmt.Errorf("decode proxy at index %d: %w", i, err)
		}
		cfg.Proxies = append(cfg.Proxies, TypedProxyConfig{
			Type:            proxyCfg.GetBaseConfig().Type,
			ProxyConfigurer: proxyCfg,
		})
	}

	return cfg, nil
}
