package schema

import (
	"fmt"
	"sort"
	"strings"
)

// Attribute describes a single attribute for a secret type.
type Attribute struct {
	Name     string
	Required bool
	Help     string
}

// SecretType describes a category of secret with its xdg:schema and attributes.
type SecretType struct {
	Name       string
	Schema     string
	Attributes []Attribute
	Help       string
}

// Types is the registry of all known secret types.
var Types = map[string]SecretType{
	"api-key": {
		Name:   "api-key",
		Schema: "io.github.nikicat.ApiKey",
		Attributes: []Attribute{
			{Name: "url", Required: true, Help: "API base URL"},
			{Name: "username", Required: false, Help: "Account username"},
		},
		Help: "API key for a web service",
	},
	"token": {
		Name:   "token",
		Schema: "io.github.nikicat.Token",
		Attributes: []Attribute{
			{Name: "url", Required: true, Help: "Service URL"},
			{Name: "username", Required: false, Help: "Account username"},
			{Name: "scope", Required: false, Help: "Token scope (comma-separated)"},
		},
		Help: "Authentication token",
	},
	"password": {
		Name:   "password",
		Schema: "io.github.nikicat.Password",
		Attributes: []Attribute{
			{Name: "url", Required: true, Help: "Service URL"},
			{Name: "username", Required: false, Help: "Account username"},
		},
		Help: "Password for a service",
	},
	"generic": {
		Name:   "generic",
		Schema: "org.freedesktop.Secret.Generic",
		Help:   "Generic secret with arbitrary key=value attributes",
	},
}

// Get looks up a secret type by name.
func Get(name string) (SecretType, bool) {
	t, ok := Types[name]
	return t, ok
}

// TypeNames returns a sorted list of all type names.
func TypeNames() []string {
	names := make([]string, 0, len(Types))
	for name := range Types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Validate checks that all required attributes are present in the provided map.
func Validate(t SecretType, provided map[string]string) error {
	var missing []string
	for _, attr := range t.Attributes {
		if attr.Required {
			if v, ok := provided[attr.Name]; !ok || v == "" {
				missing = append(missing, attr.Name)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required attribute(s): %s", strings.Join(missing, ", "))
	}
	return nil
}
