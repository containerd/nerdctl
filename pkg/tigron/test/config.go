/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package test

// WithConfig returns a config object with a certain config property set
func WithConfig(key ConfigKey, value ConfigValue) Config {
	cfg := &config{}
	cfg.Write(key, value)
	return cfg
}

// Contains the implementation of the Config interface

func configureConfig(cfg Config, parent Config) Config {
	if cfg == nil {
		cfg = &config{
			config: make(map[ConfigKey]ConfigValue),
		}
	}
	if parent != nil {
		// Note: implementation dependent
		cfg.(*config).adopt(parent)
	}
	return cfg
}

type config struct {
	config map[ConfigKey]ConfigValue
}

func (cfg *config) Write(key ConfigKey, value ConfigValue) Config {
	if cfg.config == nil {
		cfg.config = make(map[ConfigKey]ConfigValue)
	}
	cfg.config[key] = value
	return cfg
}

func (cfg *config) Read(key ConfigKey) ConfigValue {
	if cfg.config == nil {
		cfg.config = make(map[ConfigKey]ConfigValue)
	}
	if val, ok := cfg.config[key]; ok {
		return val
	}
	return ""
}

func (cfg *config) adopt(parent Config) {
	// Note: implementation dependent
	for k, v := range parent.(*config).config {
		// Only copy keys that are not set already
		if _, ok := cfg.config[k]; !ok {
			cfg.Write(k, v)
		}
	}
}
