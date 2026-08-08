package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Storage     StorageConfig     `yaml:"storage"`
	Server      ServerConfig      `yaml:"server"`
	LLM         LLMConfig         `yaml:"llm"`
	PDFService  PDFServiceConfig  `yaml:"pdf_service"`
	Translation TranslationConfig `yaml:"translation"`
}

type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

type ServerConfig struct {
	MaxUploadBytes int64 `yaml:"max_upload_bytes"`
}

type LLMConfig struct {
	DefaultProvider string              `yaml:"default_provider"`
	Providers       map[string]Provider `yaml:"providers"`
}

type Provider struct {
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// APIKey resolve o segredo do ambiente: primeiro a variável declarada em
// api_key_env, depois o fallback genérico LLM_API_KEY.
func (p Provider) APIKey() string {
	if p.APIKeyEnv != "" {
		if k := os.Getenv(p.APIKeyEnv); k != "" {
			return k
		}
	}
	return os.Getenv("LLM_API_KEY")
}

// DefaultProvider retorna o nome e a config do provider default. Se nenhum
// estiver declarado explicitamente, usa o primeiro da lista (ou "deepseek").
func (c Config) DefaultProvider() (name string, p Provider, ok bool) {
	name = c.LLM.DefaultProvider
	if name == "" {
		name = "deepseek"
	}
	p, ok = c.LLM.Providers[name]
	if !ok {
		for n, v := range c.LLM.Providers {
			name, p, ok = n, v, true
			break
		}
	}
	return name, p, ok
}

type PDFServiceConfig struct {
	URL            string   `yaml:"url"`
	RequestTimeout Duration `yaml:"request_timeout"`
}

type TranslationConfig struct {
	Concurrency    int      `yaml:"concurrency"`
	MaxRetries     int      `yaml:"max_retries"`
	RetryBackoff   Duration `yaml:"retry_backoff"`
	ChunkTokens    int      `yaml:"chunk_tokens"`
	ContextChunks  int      `yaml:"context_chunks"`
	RequestTimeout Duration `yaml:"request_timeout"`
}

// Duration aceita valores como "2m", "30s", "1h" no YAML.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = v
	return nil
}

func (c *Config) setDefaults() {
	c.Storage.DataDir = "./data"
	c.Server.MaxUploadBytes = 100 << 20
	c.LLM.DefaultProvider = "deepseek"
	c.PDFService.URL = "http://localhost:8081"
	c.PDFService.RequestTimeout.Duration = 2 * time.Minute
	c.Translation.Concurrency = 3
	c.Translation.MaxRetries = 2
	c.Translation.RetryBackoff.Duration = time.Second
	c.Translation.ChunkTokens = 4000
	c.Translation.ContextChunks = 2
	c.Translation.RequestTimeout.Duration = 3 * time.Minute
}

// Load lê config/orchestrator.yaml (caminho via CONFIG_FILE ou autodetect),
// carrega os segredos de config/secrets.env e aplica overrides de ambiente.
// Precedência: env > secrets.env > YAML > default.
func Load() (Config, error) {
	var cfg Config
	cfg.setDefaults()

	path, err := resolveConfigPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	loadSecrets(filepath.Join(filepath.Dir(path), "secrets.env"))
	applyEnvOverrides(&cfg)
	return cfg, nil
}

func resolveConfigPath() (string, error) {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return p, nil
	}
	candidates := []string{"config/orchestrator.yaml", "../config/orchestrator.yaml"}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("config file not found (tried: %s; set CONFIG_FILE to override)", strings.Join(candidates, ", "))
}

func applyEnvOverrides(c *Config) {
	if v := os.Getenv("PDF_SERVICE_URL"); v != "" {
		c.PDFService.URL = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		c.Storage.DataDir = v
	}
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.Server.MaxUploadBytes = n
		}
	}
	if os.Getenv("LLM_BASE_URL") != "" || os.Getenv("LLM_MODEL") != "" {
		name := c.LLM.DefaultProvider
		if name == "" {
			name = "deepseek"
		}
		if c.LLM.Providers == nil {
			c.LLM.Providers = map[string]Provider{}
		}
		p := c.LLM.Providers[name]
		if v := os.Getenv("LLM_BASE_URL"); v != "" {
			p.BaseURL = v
		}
		if v := os.Getenv("LLM_MODEL"); v != "" {
			p.Model = v
		}
		c.LLM.Providers[name] = p
	}
}

// loadSecrets injeta CHAVE=VALOR de secrets.env no ambiente, sem sobrescrever
// variáveis já exportadas no shell. Arquivo ausente não é erro.
func loadSecrets(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		os.Setenv(k, v)
	}
}
