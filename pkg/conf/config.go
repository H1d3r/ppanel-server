// Package conf loads the YAML configuration file into a struct, first
// filling each field that has a `default` tag with that value, so a key the
// file leaves out keeps its documented default.
package conf

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// MustLoad is Load for start-up: the process cannot run without its
// configuration, so a failure ends it.
func MustLoad(file string, v any) {
	if err := Load(file, v); err != nil {
		log.Fatalf("error: config file %s, %s", file, err.Error())
	}
}

// Load fills v, a pointer to a struct, with the defaults of its `default`
// tags and then with the YAML file.
func Load(file string, v any) error {
	setDefaults(v)
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	// Unmarshal the YAML content directly into the target structure
	if err := yaml.Unmarshal(content, v); err != nil {
		return err
	}
	return nil
}
