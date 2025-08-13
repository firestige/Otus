package config

func Load(path string) (*OtusConfig, error) {
	var config OtusConfig
	err := loadConfigFile(path, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func loadConfigFile(path string, otusConfig *OtusConfig) error {
	// Implement the logic to load the configuration file
	return nil
}
