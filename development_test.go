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
			want:    DisabledProfile,
		},
		{
			name:    "disabled",
			profile: "disabled",
			want:    DisabledProfile,
		},
		{
			name:    "none",
			profile: "none",
			want:    DisabledProfile,
		},
		{
			name:    "mem - direct",
			profile: "mem",
			want:    MemProfile,
		},
		{
			name:    "cpu - direct",
			profile: "cpu",
			want:    CPUProfile,
		},
		{
			name:    "memory + case test",
			profile: "meMorY",
			want:    MemProfile,
		},
		{
			name:    "memory",
			profile: "memory",
			want:    MemProfile,
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
