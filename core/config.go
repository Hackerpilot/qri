package core

import (
	"encoding/json"
	"fmt"

	golog "github.com/ipfs/go-log"
	"github.com/qri-io/qri/config"
	yaml "gopkg.in/yaml.v2"
)

var (
	// Config is the global configuration object
	Config *config.Config
	// ConfigFilepath is the default location for a config file
	ConfigFilepath string
)

// SaveConfig is a function that updates the configuration file
var SaveConfig = func() error {
	if err := Config.WriteToFile(ConfigFilepath); err != nil {
		return fmt.Errorf("error saving profile: %s", err)
	}
	return nil
}

// LoadConfig loads the global default configuration
func LoadConfig(path string) (err error) {
	var cfg *config.Config
	cfg, err = config.ReadFromFile(path)

	if err == nil && cfg.Profile == nil {
		err = fmt.Errorf("missing profile")
	}

	if err != nil {
		str := `couldn't read config file. error
  %s
if you've recently updated qri your config file may no longer be valid.
The easiest way to fix this is to delete your repository at:
  %s
and start with a fresh qri install by running 'qri setup' again.
Sorry, we know this is not exactly a great experience, from this point forward
we won't be shipping changes that require starting over.
`
		err = fmt.Errorf(str, err.Error(), path)
	}

	// configure logging straight away
	if cfg != nil && cfg.Logging != nil {
		for name, level := range cfg.Logging.Levels {
			golog.SetLogLevel(name, level)
		}
	}

	Config = cfg

	return err
}

// GetConfigParams are the params needed to format/specify the fields in bytes returned from the GetConfig function
type GetConfigParams struct {
	WithPrivateKey bool
	Format         string
	Concise        bool
	Field          string
}

// GetConfig returns the Config, or one of the specified fields of the Config, as a slice of bytes
// the bytes can be formatted as json, concise json, or yaml
func GetConfig(params *GetConfigParams, data *[]byte) error {
	var (
		err    error
		cfg    = &config.Config{}
		encode interface{}
	)

	*cfg = *Config

	if !params.WithPrivateKey {
		if cfg.Profile != nil {
			cfg.Profile.PrivKey = ""
		}
		if cfg.P2P != nil {
			cfg.P2P.PrivKey = ""
		}
	}

	encode = cfg

	if params.Field != "" {
		encode, err = cfg.Get(params.Field)
		if err != nil {
			return fmt.Errorf("error getting %s from config: %s", params.Field, err)
		}
	}

	switch params.Format {
	case "json":
		if params.Concise {
			*data, err = json.Marshal(encode)
		} else {
			*data, err = json.MarshalIndent(encode, "", " ")
		}
	case "yaml":
		*data, err = yaml.Marshal(encode)
	}
	if err != nil {
		return fmt.Errorf("error getting config: %s", err)
	}

	return nil
}
