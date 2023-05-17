package clio

import (
	"fmt"
	"strings"
)

const (
	CPUProfile      Profile = "cpu"
	MemProfile      Profile = "mem"
	DisabledProfile Profile = "none"
)

type Profile string

type DevelopmentConfig struct {
	Profile Profile `yaml:"profile" json:"profile"`
}

func (d *DevelopmentConfig) PostLoad() error {
	p := parseProfile(string(d.Profile))
	if p == "" {
		return fmt.Errorf("invalid profile: %q", d.Profile)
	}
	d.Profile = p
	return nil
}

func parseProfile(profile string) Profile {
	switch strings.ToLower(profile) {
	case "cpu":
		return CPUProfile
	case "mem", "memory":
		return MemProfile
	case "none", "", "disabled":
		return DisabledProfile
	default:
		return ""
	}
}
