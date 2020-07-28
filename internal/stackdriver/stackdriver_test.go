package stackdriver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuery_addFilter(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		values   []string
		expected string
	}{
		{
			name:     "single filter",
			keys:     []string{"key1"},
			values:   []string{"value1"},
			expected: `key1="value1"`,
		},
		{
			name:     "multiple filters",
			keys:     []string{"key1", "key2", "key3"},
			values:   []string{"value1", "value2", "value3"},
			expected: `key1="value1" AND key2="value2" AND key3="value3"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var q query
			for i, key := range test.keys {
				q = q.addFilter(key, test.values[i])
			}
			assert.Equal(t, test.expected, string(q))
		})
	}
}
