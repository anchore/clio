package clio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseProfile(t *testing.T) {

	tests := []struct {
		name    string
		profile string
		want    Profile
	}{
		{
			name:    "empty",
			profile: "",
			want:    ProfilingDisabled,
		},
		{
			name:    "disabled",
			profile: "disabled",
			want:    ProfilingDisabled,
		},
		{
			name:    "none",
			profile: "none",
			want:    ProfilingDisabled,
		},
		{
			name:    "mem - direct",
			profile: "mem",
			want:    ProfileMem,
		},
		{
			name:    "cpu - direct",
			profile: "cpu",
			want:    ProfileCPU,
		},
		{
			name:    "memory + case test",
			profile: "meMorY",
			want:    ProfileMem,
		},
		{
			name:    "memory",
			profile: "memory",
			want:    ProfileMem,
		},
		{
			name:    "bogus",
			profile: "bogus",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseProfile(tt.profile))
		})
	}
}
