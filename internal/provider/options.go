package provider

type Option func(*Config)

type Config struct {
	APIKey      string  `json:"api_key"`
	BaseURL     string  `json:"base_url"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`

	MaxTokens int `json:"max_tokens"`
}

const defaultTemperature = 0.7

func NewConfig(opts ...Option) Config {
	cfg := Config{
		Temperature: defaultTemperature,
	}

	Apply(&cfg, opts...)

	return cfg
}

func Apply(cfg *Config, opts ...Option) {
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
}

func (c *Config) Clone() *Config {
	clone := *c
	return &clone
}

func WithAPIKey(apiKey string) Option {
	return func(cfg *Config) {
		cfg.APIKey = apiKey
	}
}

func WithModel(model string) Option {
	return func(cfg *Config) {
		cfg.Model = model
	}
}

func WithBaseURL(baseURL string) Option {
	return func(cfg *Config) {
		cfg.BaseURL = baseURL
	}
}

func WithTemperature(temperature float64) Option {
	return func(cfg *Config) {
		cfg.Temperature = temperature
	}
}

func WithMaxTokens(maxTokens int) Option {
	return func(cfg *Config) {
		cfg.MaxTokens = maxTokens
	}
}
