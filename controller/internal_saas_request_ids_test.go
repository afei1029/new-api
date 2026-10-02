package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInternalRequestIds(t *testing.T) {
	ids, err := parseInternalRequestIds(" a, b ,a ")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, ids)

	ids, err = parseInternalRequestIds("")
	require.NoError(t, err)
	require.Nil(t, ids)

	_, err = parseInternalRequestIds("a,,b")
	require.Error(t, err)
	_, err = parseInternalRequestIds(strings.Repeat("x", 65))
	require.Error(t, err)
	_, err = parseInternalRequestIds(strings.TrimSuffix(strings.Repeat("r,", 101), ","))
	require.Error(t, err)
}
