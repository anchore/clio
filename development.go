package clio

import (
	"fmt"
	"strings"

	"github.com/anchore/fangs"
)

const (
	ProfileCPU        ProfileResource = "cpu"
	ProfileMem        ProfileResource = "mem"
	ProfilingDisabled ProfileResource = "none"
)

type ProfileResource string

type DevelopmentConfig struct {
	Profile ProfileResource `yaml:"profile" json:"profile" mapstructure:"profile"`
}

func (d *DevelopmentConfig) DescribeFields(set fangs.FieldDescriptionSet) {
	set.Add(&d.Profile, fmt.Sprintf("capture resource profiling data (available: [%s])", strings.Join([]string{string(ProfileCPU), string(ProfileMem)}, ", ")))
}

func (d *DevelopmentConfig) PostLoad() error {
	p := parseProfile(string(d.Profile))
	if p == "" {
		return fmt.Errorf("invalid profile: %q", d.Profile)
	}
	d.Profile = p
	return nil
}

func parseProfile(profile string) ProfileResource {
	switch strings.ToLower(profile) {
	case "cpu":
		return ProfileCPU
	case "mem", "memory":
		return ProfileMem
	case "none", "", "disabled":
		return ProfilingDisabled
	default:
		return ""
	}
}
