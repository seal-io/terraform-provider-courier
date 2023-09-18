package runtime

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetClasses(t *testing.T) {
	cases := []struct {
		name     string
		input    fs.FS
		expected Classes
	}{
		{
			name:  "embed classes",
			input: BuiltinSource(),
			expected: Classes{
				"docker": map[string]struct{}{
					"linux":   {},
					"windows": {},
				},
				"openjdk": map[string]struct{}{
					"linux":   {},
					"windows": {},
				},
				"tomcat": map[string]struct{}{
					"linux":   {},
					"windows": {},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := GetClasses(c.input)
			if assert.NoError(t, err, "should not return error") {
				assert.Equal(t, c.expected, actual)
			}
		})
	}
}
