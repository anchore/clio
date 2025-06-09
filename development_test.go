package clio

import (
	"fmt"
	"testing"

	"github.com/pkg/profile"
	"github.com/stretchr/testify/assert"
)

func Test_parseProfile(t *testing.T) {

	tests := []struct {
		name    string
		profile string
		want    func(*profile.Profile)
	}{
		{
			name:    "empty",
			profile: "",
			want:    nil,
		},
		{
			name:    "disabled",
			profile: "disabled",
			want:    nil,
		},
		{
			name:    "none",
			profile: "none",
			want:    nil,
		},
		{
			name:    "mem - direct",
			profile: "mem",
			want:    profile.MemProfile,
		},
		{
			name:    "cpu - direct",
			profile: "cpu",
			want:    profile.CPUProfile,
		},
		{
			name:    "memory + case test",
			profile: "meMorY",
			want:    profile.MemProfile,
		},
		{
			name:    "memory",
			profile: "memory",
			want:    profile.MemProfile,
		},
		{
			name:    "bogus",
			profile: "bogus",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			funcDesc := func(f any) string { return fmt.Sprintf("%#v", f) }
			assert.Equal(t, funcDesc(tt.want), funcDesc(profileFunc(Profile(tt.profile))))
		})
	}
}
