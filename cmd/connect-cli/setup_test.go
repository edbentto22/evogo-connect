package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupCredentialPrefersFlag(t *testing.T) {
	t.Setenv("CHATWOOT_TOKEN", "from-env")

	value, err := setupCredential("from-flag", "CHATWOOT_TOKEN")

	assert.NoError(t, err)
	assert.Equal(t, "from-flag", value)
}

func TestSetupCredentialUsesEnvironment(t *testing.T) {
	t.Setenv("EVO_INSTANCE_TOKEN", "from-env")

	value, err := setupCredential("", "EVO_INSTANCE_TOKEN")

	assert.NoError(t, err)
	assert.Equal(t, "from-env", value)
}

func TestSetupCredentialRequiresValue(t *testing.T) {
	t.Setenv("CHATWOOT_TOKEN", "")

	_, err := setupCredential("", "CHATWOOT_TOKEN")

	assert.EqualError(t, err, "credential missing: use the corresponding flag or env CHATWOOT_TOKEN")
}
