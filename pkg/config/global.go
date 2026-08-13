package config

// New returns the stable empty ledger used when config.yaml does not exist.
func New() *Config {
	cfg := &Config{}
	cfg.Library.Skills = []Skill{}
	return cfg
}
